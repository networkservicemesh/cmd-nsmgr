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

// Package test contain nsmgr tests
package test

import (
	"context"
	"io/ioutil"
	"net/url"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/networkservicemesh/sdk/pkg/tools/grpcutils"

	"github.com/networkservicemesh/api/pkg/api/registry"
	"github.com/networkservicemesh/cmd-nsmgr/internal/config"
	"github.com/networkservicemesh/cmd-nsmgr/internal/manager"
	mockReg "github.com/networkservicemesh/cmd-nsmgr/test/mock/registry"
	"github.com/sirupsen/logrus"
	"github.com/spiffe/go-spiffe/v2/spiffetls/tlsconfig"
	"github.com/spiffe/go-spiffe/v2/svid/x509svid"
	"github.com/spiffe/go-spiffe/v2/workloadapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// TempFolder creates a temporary folder for testing purposes.
func TempFolder() string {
	baseDir := path.Join(os.TempDir(), "nsm")
	err := os.MkdirAll(baseDir, os.ModeDir|os.ModePerm)
	if err != nil {
		logrus.Errorf("err: %v", err)
	}
	socketFile, _ := ioutil.TempDir(baseDir, "nsm_test")
	return socketFile
}

// IsDocker - tells we are running from inside docker or other container.
func IsDocker() bool {
	return isDockerFileExists() || isDockerHasCGroup()
}

func isDockerFileExists() bool {
	_, err := os.Stat("/.dockerenv")
	if err != nil {
		return false
	}
	return os.IsExist(err)
}

func isDockerHasCGroup() bool {
	content, err := ioutil.ReadFile("/proc/self/cgroup")
	if err != nil {
		return false
	}
	text := string(content)
	return strings.Contains(text, "docker") || strings.Contains(text, "lxc")
}

type testSetup struct {
	baseDir        string
	t              *testing.T
	registryServer mockReg.Server
	configuration  *config.Config
	ctx            context.Context
	cancel         context.CancelFunc
	Source         *workloadapi.X509Source
	SVid           *x509svid.SVID
}

const (
	setupTimeout = 150000 * time.Second
)

func (s *testSetup) init() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), setupTimeout)

	s.baseDir = TempFolder()

	// Configure ListenOnURL
	s.configuration.ListenOn = []*url.URL{{
		Scheme: "unix",
		Path:   path.Join(s.baseDir, "nsm.server.sock"),
	},
	}

	// All TCP public IP as default address
	s.configuration.ListenOn = append(s.configuration.ListenOn, &url.URL{
		Scheme: "tcp",
		Host:   "127.0.0.1:0",
	})

	var err error

	s.Source, err = workloadapi.NewX509Source(s.ctx)
	if err != nil {
		logrus.Fatalf("error getting x509 Source: %+v", err)
	}
	s.SVid, err = s.Source.GetX509SVID()
	if err != nil {
		logrus.Fatalf("error getting x509 SVid: %+v", err)
	}
	logrus.Infof("SVID: %q", s.SVid.ID)

	// Setup registry
	s.registryServer = mockReg.NewServer(s.configuration.Name, &url.URL{Scheme: "tcp", Host: "127.0.0.1:0"})

	require.Nil(s.t, s.registryServer.Start(grpc.Creds(credentials.NewTLS(tlsconfig.MTLSServerConfig(s.Source, s.Source, tlsconfig.AuthorizeAny())))))

	s.configuration.RegistryURL = s.registryServer.GetListenEndpointURI()
}

func (s *testSetup) Start() {
	s.init()

	go func() {
		e := manager.RunNsmgr(s.ctx, s.configuration)
		require.Nil(s.t, e)
	}()

	// Check Health is ok
	s.CheckHeal()
}

func (s *testSetup) Stop() {
	_ = s.Source.Close()

	s.cancel()
	_ = os.RemoveAll(s.baseDir)

	if s.registryServer != nil {
		s.registryServer.Stop()
	}
}

// newSetup construct a nsmgr used for testing.
func newSetup(t *testing.T) *testSetup {
	setup := &testSetup{
		t: t,
		configuration: &config.Config{
			Name:                 "test-nsm2",
			RegistrationInterval: 5 * time.Minute,
		},
	}
	return setup
}

func (s *testSetup) CheckHeal() {
	healthCC := s.newClient(s.ctx)

	healthClient := grpc_health_v1.NewHealthClient(healthCC)
	healthResponse, err := healthClient.Check(s.ctx, &grpc_health_v1.HealthCheckRequest{
		Service: "networkservice.NetworkService",
	})
	assert.NoError(s.t, err)
	assert.NotNil(s.t, healthResponse)
	assert.Equal(s.t, grpc_health_v1.HealthCheckResponse_SERVING, healthResponse.Status)
}

func (s *testSetup) newClient(ctx context.Context) grpc.ClientConnInterface {
	clientCtx, clientCancelFunc := context.WithTimeout(ctx, 5*time.Second)
	defer clientCancelFunc()
	grpcCC, err := grpc.DialContext(clientCtx, grpcutils.URLToTarget(s.configuration.ListenOn[0]),
		grpc.WithTransportCredentials(credentials.NewTLS(tlsconfig.MTLSClientConfig(s.Source, s.Source, tlsconfig.AuthorizeAny()))),
		grpc.WithDefaultCallOptions(grpc.WaitForReady(true)))
	require.Nil(s.t, err)
	return grpcCC
}

func (s *testSetup) NewRegistryClient(ctx context.Context) registry.NetworkServiceEndpointRegistryClient {
	grpcCC := s.newClient(ctx)
	return registry.NewNetworkServiceEndpointRegistryClient(grpcCC)
}
