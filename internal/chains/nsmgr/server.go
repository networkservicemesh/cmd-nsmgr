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

// Package nsmgr provides a Network Service Manager (nsmgrServer), but interface and implementation
package nsmgr

import (
	"context"
	"net/url"

	"github.com/networkservicemesh/api/pkg/api/networkservice"
	"github.com/networkservicemesh/api/pkg/api/registry"
	"github.com/networkservicemesh/cmd-nsmgr/internal/authz"
	"github.com/networkservicemesh/cmd-nsmgr/internal/chains/connect"
	"github.com/networkservicemesh/cmd-nsmgr/internal/chains/nsmreg"
	"github.com/networkservicemesh/sdk/pkg/networkservice/chains/client"
	"github.com/networkservicemesh/sdk/pkg/networkservice/common/authorize"
	"github.com/networkservicemesh/sdk/pkg/networkservice/common/discover"
	"github.com/networkservicemesh/sdk/pkg/networkservice/common/localbypass"
	"github.com/networkservicemesh/sdk/pkg/networkservice/common/roundrobin"
	"github.com/networkservicemesh/sdk/pkg/networkservice/core/adapters"
	adapter_registry "github.com/networkservicemesh/sdk/pkg/registry/core/adapters"
	"github.com/networkservicemesh/sdk/pkg/registry/memory"
	"github.com/networkservicemesh/sdk/pkg/tools/addressof"

	"google.golang.org/grpc"

	"github.com/networkservicemesh/sdk/pkg/networkservice/chains/endpoint"
	"github.com/networkservicemesh/sdk/pkg/tools/callback"
	"github.com/networkservicemesh/sdk/pkg/tools/token"
)

// Nsmgr - A simple combintation of the Endpoint, registry.NetworkServiceRegistryServer, and registry.NetworkServiceDiscoveryServer interfaces
type Nsmgr interface {
	endpoint.Endpoint

	registry.NetworkServiceRegistryServer
	registry.NetworkServiceDiscoveryServer
	registry.NsmRegistryServer

	callback.CallbackServiceServer
}

type nsmgrServer struct {
	endpoint.Endpoint

	registry.NetworkServiceRegistryServer
	registry.NetworkServiceDiscoveryServer
	registry.NsmRegistryServer

	storage memory.Storage
	callback.CallbackServiceServer
}

// NewServer - Creates a new Nsmgr
//           name - name of the Nsmgr
//           authzServer - authorization server chain element
//           registryCC - client connection to reach the upstream registry, could be nil, in this case only in memory storage will be used.
func NewServer(manager *registry.NetworkServiceManager, tokenGenerator token.GeneratorFunc, registryCC grpc.ClientConnInterface) Nsmgr {
	// Construct callback server
	rv := &nsmgrServer{
		CallbackServiceServer: callback.NewServer(authz.IdentityByEndpointID),
	}

	// Callback parameter factory
	callbackDialFactory := connect.WithDialOptionFactory(
		func(ctx context.Context, request *networkservice.NetworkServiceRequest, clientURL *url.URL) []grpc.DialOption {
			return []grpc.DialOption{callback.WithCallbackDialer(rv.CallbackServiceServer.(callback.Server), clientURL.String()), grpc.WithInsecure()}
		},
	)

	rv.NetworkServiceDiscoveryServer = nsmreg.NewNetworkServiceDiscoveryServer(manager, &rv.storage, registry.NewNetworkServiceDiscoveryClient(registryCC))
	rv.NsmRegistryServer = nsmreg.NewNSMRegServer(manager, &rv.storage, registry.NewNsmRegistryClient(registryCC))

	var localbypassRegistryServer registry.NetworkServiceRegistryServer

	// Construct Endpoint
	rv.Endpoint = endpoint.NewServer(
		manager.Name,
		authorize.NewServer(),
		tokenGenerator,
		discover.NewServer(adapter_registry.NewDiscoveryServerToClient(rv.NetworkServiceDiscoveryServer)),
		roundrobin.NewServer(),
		localbypass.NewServer(&localbypassRegistryServer),
		connect.NewServer(
			client.NewClientFactory(manager.Name,
				addressof.NetworkServiceClient(
					adapters.NewServerToClient(rv)),
				tokenGenerator),
			callbackDialFactory),
	)

	rv.NetworkServiceRegistryServer = nsmreg.NewNetworkServiceRegistryServer(manager, &rv.storage, localbypassRegistryServer, registry.NewNetworkServiceRegistryClient(registryCC))

	return rv
}

func (n *nsmgrServer) Register(s *grpc.Server) {
	n.Endpoint.Register(s)

	// Register callback server
	callback.RegisterCallbackServiceServer(s, n)

	// Register registry
	registry.RegisterNetworkServiceRegistryServer(s, n)
	registry.RegisterNsmRegistryServer(s, n)
	registry.RegisterNetworkServiceDiscoveryServer(s, n)
}
