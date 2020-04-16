// Copyright (c) 2020 Doc.ai and/or its affiliates.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at:
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package dockertest - is a helper tool packages to test vppagent using docker with few instances of vppagent
package dockertest

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"syscall"
	"time"

	"golang.org/x/text/runes"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

const (
	DockerTimeout = 120 * time.Second
)

// DockerTest - a base interface for docker setup of vpp agent configuration
type DockerTest interface {
	// Pull required image from docker hub
	PullImage(name string)
	Stop()
	GetContainers() []DockerContainer
	CreateContainer(name, containerImage string, cmdLine []string, config ContainerConfig) DockerContainer
}

// DockerContainer - represent a container running inside docker
type DockerContainer interface {
	// Start - start a container
	Start()
	// Stop - stop a container
	Stop()
	// GetStatus - retrieve a container status
	GetStatus() types.ContainerJSON
	// GetLogs- Get current container logs
	GetLogs() string
	// CopyToContainer -  Copy some file to container
	CopyToContainer(targetDir, targetFile, content string)

	// LogWaitPattern-  Wait for logs pattern
	LogWaitPattern(pattern string, timeout time.Duration)

	// Exec - execute command inside container
	Exec(command ...string) (string, error)
	// GetNetNS - retrieve a linux namesoaice inode container
	GetNetNS() string
	// GetID - return docker id for container
	GetID() string
	IsActive() bool
	// PrintLogs - start go routing and print all logs from container
	PrintLogs(stream bool)
}

type dockerTest struct {
	connection *client.Client
	t          require.TestingT
	containers []DockerContainer
}

// GetInode returns Inode for file
func GetInode(file string) (uint64, error) {
	fileinfo, err := os.Stat(file)
	if err != nil {
		return 0, errors.Wrap(err, "error stat file")
	}
	stat, ok := fileinfo.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, errors.New("not a stat_t")
	}
	return stat.Ino, nil
}

func (d *dockerTest) GetContainers() []DockerContainer {
	return d.containers
}

type dockerContainer struct {
	d           *dockerTest
	containerID string
	name        string
	stopped     bool
}

func (d *dockerContainer) PrintLogs(stream bool) {
	go func() {
		displayed := 0
		for d.IsActive() {
			logs := d.GetLogs()
			lines := strings.Split(logs, "\n")
			if len(lines) > displayed {
				for i := displayed; i < len(lines); i++ {
					line := removeControlCharacters(lines[i])
					logrus.Infof("Logs %v: #%v %v", d.name, i, line)
				}
			}
			displayed = len(lines)
			if !stream {
				// Exit if not a streaming
				break
			}
			time.Sleep(1 * time.Second)
		}
	}()
}

type nonASCIISet struct {
}

func (nonASCIISet) Contains(r rune) bool {
	return r < 32 || r >= 127
}

func removeControlCharacters(str string) string {
	str, _, _ = transform.String(transform.Chain(norm.NFKD, runes.Remove(nonASCIISet{})), str)
	return str
}

func (d *dockerContainer) IsActive() bool {
	return !d.stopped
}
func (d *dockerContainer) GetID() string {
	return d.containerID
}

func (d *dockerContainer) Exec(command ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), DockerTimeout)
	defer cancel()

	respID, err := d.d.connection.ContainerExecCreate(ctx, d.containerID, types.ExecConfig{
		AttachStdout: true,
		AttachStderr: true,
		Tty:          true,
		Cmd:          command,
	})
	require.Nil(d.d.t, err)

	resp, err2 := d.d.connection.ContainerExecAttach(ctx, respID.ID, types.ExecConfig{
		AttachStderr: true,
		AttachStdout: true,
		Tty:          true,
	})
	require.Nil(d.d.t, err2)
	response := ""

	for {
		select {
		case <-ctx.Done():
		default:
			line, readErr := resp.Reader.ReadString('\n')
			response += removeControlCharacters(line) + "\n"
			if readErr != nil {
				// End of read
				return strings.TrimSpace(response), nil
			}
		}
	}
}

func (d *dockerContainer) GetNetNS() string {
	link, err := d.Exec("readlink", "/proc/self/ns/net")
	require.Nil(d.d.t, err)

	pattern := regexp.MustCompile(`net:\[(.*)\]`)
	matches := pattern.FindStringSubmatch(link)
	require.True(d.d.t, len(matches) >= 1)

	return matches[1]
}

func (d *dockerContainer) LogWaitPattern(pattern string, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), DockerTimeout)
	defer cancel()

	r, err := regexp.Compile(pattern)
	if err != nil {
		require.Failf(d.d.t, "failed to compile pattern: %v %v", pattern, err)
	}
	matcher := func(s string) bool {
		return r.FindStringSubmatch(s) != nil
	}

	for {
		curLogs := d.GetLogs()
		lines := strings.Split(curLogs, "\n")
		nl := ""
		for _, line := range lines {
			// trim non ansi
			line = removeControlCharacters(line)
			if len(line) > 0 {
				nl = line
			}
			if matcher(nl) {
				return
			}
		}
		// Find pattern
		select {
		case <-ctx.Done():
			require.Failf(d.d.t, "Timeout waiting for pattern %v %v\n Logs:", d.containerID, pattern, curLogs)
			return
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func generate(name, content string) (io.Reader, error) {
	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)
	hdr := &tar.Header{
		Name: name,
		Size: int64(len(content)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return nil, err
	}
	if _, err := tw.Write([]byte(content)); err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	return buf, nil
}

func (d *dockerContainer) CopyToContainer(targetDir, targetFile, content string) {
	ctx, cancel := context.WithTimeout(context.Background(), DockerTimeout)
	defer cancel()

	tarFile, err := generate(targetFile, content)
	require.Nil(d.d.t, err)

	err = d.d.connection.CopyToContainer(ctx, d.containerID, targetDir, tarFile, types.CopyToContainerOptions{
		AllowOverwriteDirWithFile: true,
	})
	require.Nil(d.d.t, err)
}

func (d *dockerContainer) GetStatus() types.ContainerJSON {
	ctx, cancel := context.WithTimeout(context.Background(), DockerTimeout)
	defer cancel()
	info, err := d.d.connection.ContainerInspect(ctx, d.containerID)
	require.Nil(d.d.t, err)
	return info
}

func (d *dockerContainer) Stop() {
	d.d.stopContainer(d.name, d.containerID)
	d.stopped = true
}

func (d *dockerTest) stopContainer(name, containerID string) {
	logrus.Infof("Stopping container: %v", name)
	ctx, cancel := context.WithTimeout(context.Background(), DockerTimeout)
	defer cancel()

	timeout := 0 * time.Millisecond

	info, err := d.connection.ContainerInspect(ctx, containerID)
	if err != nil {
		logrus.Errorf("Failed to get container information %v", err)
	}
	if info.ContainerJSONBase != nil && info.State != nil && info.State.Running {
		err = d.connection.ContainerStop(ctx, containerID, &timeout)
		if err != nil {
			logrus.Errorf("failed to stop container %v", err)
		}
	}
	logrus.Infof("container stopped %v %v", name, containerID)
}

func (d *dockerContainer) Start() {
	ctx, cancel := context.WithTimeout(context.Background(), DockerTimeout)
	defer cancel()
	err := d.d.connection.ContainerStart(ctx, d.containerID, types.ContainerStartOptions{})
	require.Nil(d.d.t, err)

	info := types.ContainerJSON{}
	for {
		select {
		case <-ctx.Done():
			require.Failf(d.d.t, "Failed to wait for container running state %v %v", d.name, d.containerID)
			return
		case <-time.After(10 * time.Millisecond):
		}
		info = d.GetStatus()
		curLogs := d.GetLogs()
		logrus.Infof("Staring logs:" + curLogs)
		if info.State != nil && info.State.Running {
			// Container is running all is ok
			break
		}
	}
	require.NotNil(d.d.t, info.State)
	require.Equal(d.d.t, true, info.State.Running)

	logrus.Infof("Status of container creation: %v", d.GetLogs())
}

func (d *dockerContainer) GetLogs() string {
	ctx, cancel := context.WithTimeout(context.Background(), DockerTimeout)
	defer cancel()
	reader, err := d.d.connection.ContainerLogs(ctx, d.containerID, types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Details:    true,
	})
	require.Nil(d.d.t, err)

	s := ""
	out := bytes.NewBufferString(s)
	_, _ = io.Copy(out, reader)
	_ = reader.Close()
	s = out.String()
	ss := strings.Split(s, "\n")
	s = ""
	for _, sss := range ss {
		s += strings.TrimSpace(sss) + "\n"
	}
	return s
}

func (d *dockerTest) Stop() {
	// Close all created containers
	for _, cnt := range d.containers {
		cnt.Stop()
	}
	err := d.connection.Close()
	require.Nil(d.t, err)
}

func conf(name string, options ...string) string {
	result := name + " {\n"
	for _, opt := range options {
		result += fmt.Sprintf("\t%s\n", opt)
	}
	result += "}\n"
	return result
}

func (d *dockerTest) PullImage(name string) {
	logrus.Infof("Fetching docker image: %v", name)
	ctx, cancel := context.WithTimeout(context.Background(), DockerTimeout)
	defer cancel()
	reader, err := d.connection.ImagePull(ctx, name, types.ImagePullOptions{})
	require.Nil(d.t, err)
	s := ""
	out := bytes.NewBufferString(s)
	_, _ = io.Copy(out, reader)
	_ = reader.Close()
	s = out.String()
	ss := strings.Split(s, "\n")
	s = ""
	for _, sss := range ss {
		s += strings.TrimSpace(sss) + "\n"
	}
	logrus.Infof("Docker output:\n %v", s)
}

// ContainerConfig - some configurations for container created.
type ContainerConfig struct {
	Privileged   bool
	PortBindings nat.PortMap
	ExposedPorts nat.PortSet
}

func (d *dockerTest) CreateContainer(name, containerImage string, cmdLine []string, config ContainerConfig) DockerContainer {
	logrus.Infof("Creating docker container: %v %v", name, cmdLine)
	ctx, cancel := context.WithTimeout(context.Background(), DockerTimeout)
	defer cancel()

	filterValue := filters.NewArgs()
	filterValue.Add("label", "docker_test_container="+name)
	containers, err := d.connection.ContainerList(ctx, types.ContainerListOptions{
		Filters: filterValue,
	})
	require.Nil(d.t, err)

	for i := 0; i < len(containers); i++ {
		d.stopContainer(name, containers[i].ID)
	}

	resp, err := d.connection.ContainerCreate(ctx, &container.Config{
		Image:        containerImage,
		Cmd:          cmdLine,
		ExposedPorts: config.ExposedPorts,
		Labels: map[string]string{
			"docker_test_container": name,
		},
	}, &container.HostConfig{
		Privileged:   config.Privileged,
		PidMode:      "host",
		PortBindings: config.PortBindings,
	}, nil, "")
	require.NotNil(d.t, resp)
	require.Nil(d.t, err)
	result := &dockerContainer{
		name:        name,
		d:           d,
		containerID: resp.ID,
	}

	d.containers = append(d.containers, result)

	return result
}

// NewDockerTest - creates a docker testing helper infrastructure
func NewDockerTest(t require.TestingT) DockerTest {
	cli, err := client.NewEnvClient()
	require.Nil(t, err)

	return &dockerTest{
		t:          t,
		connection: cli,
	}
}
