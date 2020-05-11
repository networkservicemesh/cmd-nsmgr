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

package registry

import (
	"context"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/networkservicemesh/api/pkg/api/registry"
	"github.com/networkservicemesh/sdk/pkg/registry/core/next"
)

type endpointChannelServer struct {
	channel chan *registry.NetworkServiceEndpoint
}

func newNSEChainServer(channel chan *registry.NetworkServiceEndpoint) registry.NetworkServiceRegistryServer {
	return &endpointChannelServer{
		channel: channel,
	}
}

func (e *endpointChannelServer) RegisterNSE(ctx context.Context, registration *registry.NSERegistration) (*registry.NSERegistration, error) {
	result, err := next.NetworkServiceRegistryServer(ctx).RegisterNSE(ctx, registration)
	if err == nil {
		e.channel <- result.NetworkServiceEndpoint
	}
	return result, err
}

func (e *endpointChannelServer) BulkRegisterNSE(server registry.NetworkServiceRegistry_BulkRegisterNSEServer) error {
	return next.NetworkServiceRegistryServer(server.Context()).BulkRegisterNSE(server)
}

func (e *endpointChannelServer) RemoveNSE(ctx context.Context, request *registry.RemoveNSERequest) (*empty.Empty, error) {
	return next.NetworkServiceRegistryServer(ctx).RemoveNSE(ctx, request)
}

type nsmChannelServer struct {
	channel chan *registry.NetworkServiceManager
}

func newNSMChainServer(channel chan *registry.NetworkServiceManager) registry.NsmRegistryServer {
	return &nsmChannelServer{
		channel: channel,
	}
}

func (n *nsmChannelServer) RegisterNSM(ctx context.Context, manager *registry.NetworkServiceManager) (*registry.NetworkServiceManager, error) {
	result, err := next.NSMRegistryServer(ctx).RegisterNSM(ctx, manager)
	if err == nil {
		n.channel <- result
	}
	return result, err
}

func (n *nsmChannelServer) GetEndpoints(ctx context.Context, empt *empty.Empty) (*registry.NetworkServiceEndpointList, error) {
	return next.NSMRegistryServer(ctx).GetEndpoints(ctx, empt)
}
