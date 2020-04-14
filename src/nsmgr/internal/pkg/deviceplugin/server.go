// Copyright (c) 2020 Cisco and/or its affiliates.
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

package deviceplugin

import (
	"context"
	"fmt"
	"github.com/networkservicemesh/cmd-nsmgr/src/nsmgr/internal/pkg/constants"
	"github.com/networkservicemesh/cmd-nsmgr/src/nsmgr/internal/pkg/flags"
	"github.com/networkservicemesh/sdk/pkg/tools/serialize"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
	"net"
	"os"
	"path"
	"path/filepath"
	"time"
)

type NsmDevicePluginServer interface {
	pluginapi.DevicePluginServer
	Start() error
	Stop()
	ListenEndpoint() string
}

type nsmgrDevicePlugin struct {
	devices               map[string]*pluginapi.Device
	allocatedDevices      map[string]*pluginapi.Device
	executor              serialize.Executor
	insecure              bool
	listAndWatchListeners []pluginapi.DevicePlugin_ListAndWatchServer
	grpcServer            *grpc.Server
	listenEndpoint        string
	sock                  net.Listener
}

func (n *nsmgrDevicePlugin) ListenEndpoint() string {
	return n.listenEndpoint
}
func (n *nsmgrDevicePlugin) Stop() {
	if n.sock != nil {
		_ = n.sock.Close()
		n.sock = nil
	}
	if n.grpcServer != nil {
		n.grpcServer.Stop()
		n.grpcServer = nil
	}
}
func (n *nsmgrDevicePlugin) Start() error {
	// We do not need a peer tracker in case of single source GRPC server.
	_ = os.Remove(n.listenEndpoint)
	var err error
	n.sock, err = net.Listen("unix", n.listenEndpoint)
	if err != nil {
		err = errors.WithMessagef(err, "failed to listen on %s: %+v", n.listenEndpoint, err)
		return err
	}

	n.grpcServer = grpc.NewServer()
	pluginapi.RegisterDevicePluginServer(n.grpcServer, n)
	go func() {
		if serveErr := n.grpcServer.Serve(n.sock); serveErr != nil {
			logrus.Errorf("failed to start device plugin grpc server %v %v", n.listenEndpoint, serveErr)
		}
	}()

	return n.Register()
}

func NewServer(insecure bool) NsmDevicePluginServer {
	rv := &nsmgrDevicePlugin{
		devices:          make(map[string]*pluginapi.Device, constants.DeviceBuffer),
		allocatedDevices: make(map[string]*pluginapi.Device, constants.DeviceBuffer),
		executor:         serialize.NewExecutor(),
		insecure:         insecure,
		listenEndpoint:   flags.Values.DeviceAPIListenEndpoint,
	}
	// TODO - Fix applying peer_tracker here
	// rv.Nsmgr = peer_tracker.NewServer(nsmgr.NewEndpoint(name, registryCC), &rv.reallocate)
	rv.resizeDevicePool()
	return rv
}

func (n *nsmgrDevicePlugin) ListAndWatch(e *pluginapi.Empty, s pluginapi.DevicePlugin_ListAndWatchServer) error {
	n.executor.AsyncExec(func() {
		n.listAndWatchListeners = append(n.listAndWatchListeners, s)
		listAndWatchResponse := &pluginapi.ListAndWatchResponse{}
		for _, device := range n.devices {
			listAndWatchResponse.Devices = append(listAndWatchResponse.Devices, device)
		}
		for _, listAndWatchListener := range n.listAndWatchListeners {
			_ = listAndWatchListener.Send(listAndWatchResponse)
		}
	})

	<-s.Context().Done()
	n.executor.AsyncExec(func() {
		var listAndWatchListeners []pluginapi.DevicePlugin_ListAndWatchServer
		for _, listAndWatchListener := range n.listAndWatchListeners {
			if listAndWatchListener != s {
				listAndWatchListeners = append(listAndWatchListeners, listAndWatchListener)
			}
		}
	})
	return nil
}

func (n *nsmgrDevicePlugin) Allocate(ctx context.Context, reqs *pluginapi.AllocateRequest) (*pluginapi.AllocateResponse, error) {
	containerResponses := []*pluginapi.ContainerAllocateResponse{}
	for _, req := range reqs.GetContainerRequests() {
		for _, deviceid := range req.GetDevicesIDs() {
			_, ok := n.devices[deviceid]
			if !ok {
				return nil, fmt.Errorf("device id passed not found %v", deviceid)
			}
			// Clean any memif files, or endpoint socket files.
			// Connections will be closed automatically.
			n.cleanFolder(ctx, hostDeviceDirectory(deviceid))
			_ = os.MkdirAll(hostDeviceDirectory(deviceid), os.ModeDir|os.ModePerm)

			mounts := []*pluginapi.Mount{
				{
					ContainerPath: containerServerDirectory(deviceid),
					HostPath:      hostServerDirectory(deviceid),
					ReadOnly:      false,
				},
				{
					ContainerPath: containerDeviceDirectory(deviceid),
					HostPath:      hostDeviceDirectory(deviceid),
					ReadOnly:      false,
				},
			}
			envs := map[string]string{
				constants.NsmServerSocketEnv: containerServerSocketFile(deviceid),
				constants.NsmClientSocketEnv: containerClientSocketFile(deviceid),
			}
			if !n.insecure {
				mounts = append(mounts, &pluginapi.Mount{
					ContainerPath: constants.SpireSocket,
					HostPath:      constants.SpireSocket,
					ReadOnly:      true,
				})
			}
			containerResponse := &pluginapi.ContainerAllocateResponse{
				Envs:   envs,
				Mounts: mounts,
			}
			containerResponses = append(containerResponses, containerResponse)
			n.executor.AsyncExec(func() {
				n.allocatedDevices[deviceid] = n.devices[deviceid]
			})
		}
	}
	n.resizeDevicePool()
	return &pluginapi.AllocateResponse{ContainerResponses: containerResponses}, nil
}

func (n *nsmgrDevicePlugin) PreStartContainer(context.Context, *pluginapi.PreStartContainerRequest) (*pluginapi.PreStartContainerResponse, error) {
	return &pluginapi.PreStartContainerResponse{}, nil
}

func (n *nsmgrDevicePlugin) GetDevicePluginOptions(context.Context, *pluginapi.Empty) (*pluginapi.DevicePluginOptions, error) {
	return &pluginapi.DevicePluginOptions{}, nil
}

func (n *nsmgrDevicePlugin) resizeDevicePool() {
	n.executor.AsyncExec(func() {
		for len(n.devices)-len(n.allocatedDevices) < constants.DeviceBuffer {
			device := &pluginapi.Device{
				ID:     "nsm-" + fmt.Sprintf("%d", len(n.devices)),
				Health: pluginapi.Healthy,
			}
			n.devices[device.GetID()] = device
		}
		listAndWatchResponse := &pluginapi.ListAndWatchResponse{}

		for _, d := range n.devices {
			listAndWatchResponse.Devices = append(listAndWatchResponse.Devices, d)
		}
		for _, listAndWatchListener := range n.listAndWatchListeners {
			_ = listAndWatchListener.Send(listAndWatchResponse)
		}
	})
}

func (n *nsmgrDevicePlugin) cleanFolder(ctx context.Context, dir string) {
	// Clean a passed folder
	d, err := os.Open(dir)
	if err != nil {
		// folder not exists, return
		return
	}
	defer func() { _ = d.Close() }()
	names := []string{}
	names, err = d.Readdirnames(-1)
	if err != nil {
		// folder not exists, return
		return
	}
	for _, name := range names {
		fp := filepath.Join(dir, name)
		err = os.RemoveAll(fp)
		if err != nil {
			logrus.Errorf("failed to remove all at %v %v", fp, err)
		}
	}
}

func (n *nsmgrDevicePlugin) Register() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, flags.Values.DeviceAPIRegistryServer, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		return err
	}
	defer func() {
		_ = conn.Close()
	}()

	client := pluginapi.NewRegistrationClient(conn)
	reqt := &pluginapi.RegisterRequest{
		Version:      pluginapi.Version,
		Endpoint:     path.Base(n.listenEndpoint),
		ResourceName: constants.ResourceName,
	}

	_, err = client.Register(context.Background(), reqt)
	if err != nil {
		return err
	}
	return nil
}
