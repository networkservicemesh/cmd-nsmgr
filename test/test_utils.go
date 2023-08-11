// Copyright (c) 2020-2021 Doc.ai and/or its affiliates.
//
// Copyright (c) 2022 Cisco and/or its affiliates.
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
	"net/url"
	"os"
	"path"
	"testing"
	"time"

	"github.com/edwarnicke/grpcfd"

	"github.com/networkservicemesh/sdk/pkg/registry/common/grpcmetadata"
	"github.com/networkservicemesh/sdk/pkg/registry/core/next"
	"github.com/networkservicemesh/sdk/pkg/tools/grpcutils"
	"github.com/networkservicemesh/sdk/pkg/tools/log"
	"github.com/networkservicemesh/sdk/pkg/tools/log/logruslogger"
	"github.com/networkservicemesh/sdk/pkg/tools/spiffejwt"
	"github.com/networkservicemesh/sdk/pkg/tools/token"

	"github.com/sirupsen/logrus"
	"github.com/spiffe/go-spiffe/v2/spiffetls/tlsconfig"
	"github.com/spiffe/go-spiffe/v2/svid/x509svid"
	"github.com/spiffe/go-spiffe/v2/workloadapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health/grpc_health_v1"

	registryapi "github.com/networkservicemesh/api/pkg/api/registry"

	"github.com/networkservicemesh/cmd-nsmgr/internal/config"
	"github.com/networkservicemesh/cmd-nsmgr/internal/manager"
	mockReg "github.com/networkservicemesh/cmd-nsmgr/test/mock/registry"
)

// TempFolder creates a temporary folder for testing purposes.
func TempFolder() string {
	baseDir := path.Join(os.TempDir(), "nsm")
	err := os.MkdirAll(baseDir, os.ModeDir|os.ModePerm)
	if err != nil {
		logrus.Errorf("err: %v", err)
	}
	socketFile, _ := os.MkdirTemp(baseDir, "nsm_test")
	return socketFile
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

func (s *testSetup) init() {
	s.ctx, s.cancel = context.WithCancel(context.Background())

	s.ctx = log.WithLog(s.ctx, logruslogger.New(s.ctx, map[string]interface{}{"cmd": "NsmgrTestSetup"}))

	s.baseDir = TempFolder()

	// Configure ListenOnURL
	s.configuration.ListenOn = []url.URL{{
		Scheme: "unix",
		Path:   path.Join(s.baseDir, "nsm.server.sock"),
	},
	}

	// All TCP public IP as default address
	s.configuration.ListenOn = append(s.configuration.ListenOn, url.URL{
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
	s.registryServer = mockReg.NewServer(
		&url.URL{Scheme: "tcp", Host: "127.0.0.1:0"},
		spiffejwt.TokenGeneratorFunc(s.Source, time.Hour))

	require.Nil(s.t, s.registryServer.Start(grpc.Creds(credentials.NewTLS(tlsconfig.MTLSServerConfig(s.Source, s.Source, tlsconfig.AuthorizeAny())))))

	s.configuration.RegistryURL = *s.registryServer.GetListenEndpointURI()
	s.configuration.MaxTokenLifetime = time.Hour
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
			Name:                        "nsmgr",
			ForwarderNetworkServiceName: "forwarder",
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
	grpcCC, err := grpc.DialContext(clientCtx, grpcutils.URLToTarget(&s.configuration.ListenOn[0]), s.dialOptions()...)
	require.Nil(s.t, err)
	return grpcCC
}

func (s *testSetup) dialOptions() []grpc.DialOption {
	return []grpc.DialOption{
		grpc.WithTransportCredentials(manager.GrpcfdTransportCredentials(credentials.NewTLS(tlsconfig.MTLSClientConfig(s.Source, s.Source, tlsconfig.AuthorizeAny())))),
		grpc.WithDefaultCallOptions(
			grpc.PerRPCCredentials(token.NewPerRPCCredentials(spiffejwt.TokenGeneratorFunc(s.Source, s.configuration.MaxTokenLifetime))),
		),
		grpcfd.WithChainStreamInterceptor(),
		grpcfd.WithChainUnaryInterceptor(),
	}
}

func (s *testSetup) NewRegistryClient(ctx context.Context) registryapi.NetworkServiceEndpointRegistryClient {
	grpcCC := s.newClient(ctx)
	return next.NewNetworkServiceEndpointRegistryClient(
		grpcmetadata.NewNetworkServiceEndpointRegistryClient(),
		registryapi.NewNetworkServiceEndpointRegistryClient(grpcCC))
}
