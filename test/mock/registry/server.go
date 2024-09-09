// Copyright (c) 2020-2021 Doc.ai and/or its affiliates.
//
// Copyright (c) 2022-2024 Cisco and/or its affiliates.
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

// Package registry contains mock in memory registry hosted on tcp
package registry

import (
	"context"
	"net/url"
	"os"

	"github.com/networkservicemesh/sdk/pkg/tools/log"
	"github.com/networkservicemesh/sdk/pkg/tools/token"

	"github.com/edwarnicke/serialize"

	"github.com/networkservicemesh/sdk/pkg/tools/grpcutils"
	"github.com/networkservicemesh/sdk/pkg/tools/log/logruslogger"

	"github.com/sirupsen/logrus"

	"github.com/networkservicemesh/sdk/pkg/registry/common/authorize"
	"github.com/networkservicemesh/sdk/pkg/registry/common/grpcmetadata"
	"github.com/networkservicemesh/sdk/pkg/registry/common/memory"
	"github.com/networkservicemesh/sdk/pkg/registry/common/updatepath"
	"github.com/networkservicemesh/sdk/pkg/registry/core/chain"

	"google.golang.org/grpc"

	"github.com/networkservicemesh/api/pkg/api/registry"
)

// Server mock registry server interface.
type Server interface {
	NetworkServiceRegistryServer() registry.NetworkServiceRegistryServer
	NetworkServiceEndpointRegistryServer() registry.NetworkServiceEndpointRegistryServer

	Stop()
	Start(options ...grpc.ServerOption) error
	GetListenEndpointURI() *url.URL
}

type serverImpl struct {
	listenOn *url.URL
	server   *grpc.Server
	executor serialize.Executor

	nsServer  registry.NetworkServiceRegistryServer
	nseServer registry.NetworkServiceEndpointRegistryServer

	ctx     context.Context
	cancel  context.CancelFunc
	errChan <-chan error
}

func (s *serverImpl) NetworkServiceRegistryServer() registry.NetworkServiceRegistryServer {
	return s.nsServer
}

func (s *serverImpl) NetworkServiceEndpointRegistryServer() registry.NetworkServiceEndpointRegistryServer {
	return s.nseServer
}

func (s *serverImpl) GetListenEndpointURI() *url.URL {
	return s.listenOn
}

// NewServer - created a mock kubelet server to perform testing.
func NewServer(listenOn *url.URL, tokenGenerator token.GeneratorFunc) Server {
	result := &serverImpl{
		listenOn: listenOn,
		executor: serialize.Executor{},
	}
	result.nsServer = chain.NewNetworkServiceRegistryServer(
		grpcmetadata.NewNetworkServiceRegistryServer(),
		updatepath.NewNetworkServiceRegistryServer(tokenGenerator),
		authorize.NewNetworkServiceRegistryServer(),
		memory.NewNetworkServiceRegistryServer())
	result.nseServer = chain.NewNetworkServiceEndpointRegistryServer(
		grpcmetadata.NewNetworkServiceEndpointRegistryServer(),
		updatepath.NewNetworkServiceEndpointRegistryServer(tokenGenerator),
		authorize.NewNetworkServiceEndpointRegistryServer(),
		memory.NewNetworkServiceEndpointRegistryServer())

	return result
}

func (s *serverImpl) Start(options ...grpc.ServerOption) error {
	s.server = grpc.NewServer(options...)

	registry.RegisterNetworkServiceRegistryServer(s.server, s.NetworkServiceRegistryServer())
	registry.RegisterNetworkServiceEndpointRegistryServer(s.server, s.NetworkServiceEndpointRegistryServer())

	if s.listenOn.Scheme == "unix" {
		_ = os.Remove(s.listenOn.Path)
	}

	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.ctx = log.WithLog(s.ctx, logruslogger.New(s.ctx, map[string]interface{}{"cmd": "NsmgrMockRegistry"}))

	s.errChan = grpcutils.ListenAndServe(s.ctx, s.listenOn, s.server)

	logrus.Infof("Mock registry host at: %v", s.GetListenEndpointURI().String())

	go func() {
		select {
		case <-s.ctx.Done():
			break
		case e := <-s.errChan:
			if e != nil {
				logrus.Errorf("err: %v", e)
			}
		}
	}()
	return nil
}

func (s *serverImpl) Stop() {
	s.cancel()
	s.server.Stop()
}
