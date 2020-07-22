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

// Package test contains local nsmgr tests.
package test

import (
	"context"
	"net/url"
	"time"

	"github.com/networkservicemesh/sdk/pkg/networkservice/chains/endpoint"
	"github.com/networkservicemesh/sdk/pkg/networkservice/common/authorize"
	"github.com/networkservicemesh/sdk/pkg/networkservice/common/setextracontext"
	"github.com/networkservicemesh/sdk/pkg/tools/grpcutils"

	"github.com/spiffe/go-spiffe/v2/spiffetls/tlsconfig"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/networkservicemesh/api/pkg/api/networkservice"
	"github.com/networkservicemesh/api/pkg/api/networkservice/mechanisms/cls"
	"github.com/networkservicemesh/api/pkg/api/networkservice/mechanisms/kernel"
	"github.com/networkservicemesh/api/pkg/api/registry"
	"github.com/networkservicemesh/cmd-nsmgr/internal/authz"
	"github.com/networkservicemesh/sdk/pkg/networkservice/chains/client"
	"github.com/networkservicemesh/sdk/pkg/tools/callback"
	"github.com/networkservicemesh/sdk/pkg/tools/spiffejwt"
	"github.com/stretchr/testify/require"
)

func serve(ctx context.Context, listenOn *url.URL, e endpoint.Endpoint, opt ...grpc.ServerOption) (<-chan error, *grpc.Server) {
	server := grpc.NewServer(opt...)
	e.Register(server)

	return grpcutils.ListenAndServe(ctx, listenOn, server), server
}

// Check endpoint registration and Client request to it with callback
func (f *NsmgrTestSuite) TestNSmgrEndpointCallback() {
	t := f.T()
	// TODO: check with defer goleak.VerifyNone(t)
	setup := newSetup(t)
	setup.Start()
	defer setup.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()

	nsmClient := setup.newClient(ctx)

	nseURL := &url.URL{Scheme: "tcp", Host: "127.0.0.1:0"}

	nseErr, nseGRPC := serve(ctx, nseURL,
		endpoint.NewServer(ctx, "nse", authorize.NewServer(), spiffejwt.TokenGeneratorFunc(setup.Source, setup.configuration.MaxTokenLifetime), setextracontext.NewServer(map[string]string{"perform": "ok"})),
		grpc.Creds(credentials.NewTLS(tlsconfig.MTLSServerConfig(setup.Source, setup.Source, tlsconfig.AuthorizeAny()))))

	require.NotNil(t, nseErr)
	require.NotNil(t, nseGRPC)

	// Serve callbacks
	callbackClient := callback.NewClient(nsmClient, nseGRPC)
	// Construct context to pass identity to server.
	callbackClient.Serve(authz.WithCallbackEndpointID(ctx, nseURL))

	nsRegClient := registry.NewNetworkServiceRegistryClient(nsmClient)
	regClient := registry.NewNetworkServiceEndpointRegistryClient(nsmClient)
	ns, _ := nsRegClient.Register(context.Background(), &registry.NetworkService{
		Name: "my-service",
	})

	nseReg, err := regClient.Register(context.Background(), &registry.NetworkServiceEndpoint{
		NetworkServiceNames: []string{ns.Name},
		Url:                 "callback:" + nseURL.String(),
	})

	require.Nil(t, err)
	require.NotNil(t, nseReg)

	cl := client.NewClient(context.Background(), "nsc-1", nil, spiffejwt.TokenGeneratorFunc(setup.Source, setup.configuration.MaxTokenLifetime), nsmClient)

	var connection *networkservice.Connection

	connection, err = cl.Request(ctx, &networkservice.NetworkServiceRequest{
		MechanismPreferences: []*networkservice.Mechanism{
			{Cls: cls.LOCAL, Type: kernel.MECHANISM},
		},
		Connection: &networkservice.Connection{
			Id:             "1",
			NetworkService: "my-service",
			Context:        &networkservice.ConnectionContext{},
		},
	})
	require.Nil(t, err)
	require.NotNil(t, connection)
	require.Equal(t, 3, len(connection.Path.PathSegments))

	_, err = cl.Close(ctx, connection)
	require.Nil(t, err)
}
