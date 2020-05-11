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

// Package registry contains mock in memory registry hosted on tcp
package registry

import (
	"fmt"
	"net"
	"net/url"
	"os"

	"github.com/networkservicemesh/sdk/pkg/registry/core/chain"
	"github.com/networkservicemesh/sdk/pkg/registry/memory"
	"github.com/sirupsen/logrus"

	"github.com/networkservicemesh/api/pkg/api/registry"
	"github.com/networkservicemesh/sdk/pkg/tools/serialize"
	"google.golang.org/grpc"
)

// Server mock registry server interface
type Server interface {
	registry.NsmRegistryServer
	registry.NetworkServiceRegistryServer

	Stop()
	Start(options ...grpc.ServerOption) error
	GetListenEndpointURI() *url.URL

	GetNSMChannel() chan *registry.NetworkServiceManager
	GetEndpointChannel() chan *registry.NetworkServiceEndpoint
}

type serverImpl struct {
	listenOn *url.URL
	server   *grpc.Server
	executor serialize.Executor
	listener net.Listener

	registry.NetworkServiceRegistryServer
	registry.NsmRegistryServer

	nsmChannel      chan *registry.NetworkServiceManager
	endpointChannel chan *registry.NetworkServiceEndpoint
	storage         *memory.Storage
	registry.NetworkServiceDiscoveryServer
}

func (s *serverImpl) GetListenEndpointURI() *url.URL {
	addr := s.listener.Addr()
	if tcpAddr, ok := addr.(*net.TCPAddr); ok {
		if tcpAddr.IP.IsUnspecified() {
			return &url.URL{Scheme: addr.Network(), Path: fmt.Sprintf("127.0.0.1:%v", tcpAddr.Port)}
		}
	}
	return &url.URL{Scheme: addr.Network(), Path: addr.String()}
}

func (s *serverImpl) GetNSMChannel() chan *registry.NetworkServiceManager {
	return s.nsmChannel
}
func (s *serverImpl) GetEndpointChannel() chan *registry.NetworkServiceEndpoint {
	return s.endpointChannel
}

// NewServer - created a mock kubelet server to perform testing.
func NewServer(name string, listenOn *url.URL) Server {
	storage := &memory.Storage{}
	result := &serverImpl{
		listenOn:        listenOn,
		storage:         storage,
		executor:        serialize.Executor{},
		nsmChannel:      make(chan *registry.NetworkServiceManager, 100),
		endpointChannel: make(chan *registry.NetworkServiceEndpoint, 100),
	}
	result.NetworkServiceRegistryServer = chain.NewNetworkServiceRegistryServer(memory.NewNetworkServiceRegistryServer(storage), newNSEChainServer(result.endpointChannel))
	result.NsmRegistryServer = chain.NewNSMRegistryServer(memory.NewNSMRegistryServer(name, storage), newNSMChainServer(result.nsmChannel))
	result.NetworkServiceDiscoveryServer = memory.NewNetworkServiceDiscoveryServer(storage)
	return result
}

func (s *serverImpl) Start(options ...grpc.ServerOption) error {
	s.server = grpc.NewServer(options...)

	registry.RegisterNetworkServiceRegistryServer(s.server, s)
	registry.RegisterNsmRegistryServer(s.server, s)
	registry.RegisterNetworkServiceDiscoveryServer(s.server, s)

	if s.listenOn.Scheme == "unix" {
		_ = os.Remove(s.listenOn.Path)
	}
	var err error
	s.listener, err = net.Listen(s.listenOn.Scheme, s.listenOn.Path)
	if err != nil {
		return err
	}
	logrus.Infof("Mock registry host at: %v", s.GetListenEndpointURI().String())
	go func() {
		e := s.server.Serve(s.listener)
		if e != nil {
			logrus.Errorf("err: %v", e)
		}
	}()
	return nil
}

func (s *serverImpl) Stop() {
	s.server.Stop()
	_ = s.listener.Close()
}
