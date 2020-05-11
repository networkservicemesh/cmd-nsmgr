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

	"github.com/networkservicemesh/api/pkg/api/networkservice"
	"github.com/networkservicemesh/api/pkg/api/networkservice/mechanisms/cls"
	"github.com/networkservicemesh/api/pkg/api/networkservice/mechanisms/kernel"
	"github.com/networkservicemesh/api/pkg/api/registry"
	"github.com/networkservicemesh/cmd-nsmgr/internal/authz"
	"github.com/networkservicemesh/cmd-nsmgr/test/mock/callbacknse"
	"github.com/networkservicemesh/sdk/pkg/networkservice/chains/client"
	"github.com/networkservicemesh/sdk/pkg/tools/callback"
	"github.com/networkservicemesh/sdk/pkg/tools/spiffejwt"
	"github.com/stretchr/testify/require"
)

// Check endpoint registration and Client request to it with callback
func (f *NsmgrTestSuite) TestNSmgrEndpointCallback() {
	t := f.T()
	setup := newSetup(t)
	setup.Start()
	defer setup.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()

	nsmClient := setup.newClient(ctx)

	nseURL := &url.URL{Scheme: "tcp", Host: "127.0.0.1:0"}
	nseServer, nseErr := callbacknse.NewNSE(ctx, nseURL, func(request *networkservice.NetworkServiceRequest) {
		// Update labels to be sure all code is executed.
		request.GetConnection().Labels = map[string]string{"perform": "ok"}
	})
	require.NotNil(t, nseErr)

	// Serve callbacks
	callbackClient := callback.NewClient(nsmClient, nseServer)
	// Construct context to pass identity to server.
	callbackClient.Serve(authz.WithCallbackEndpointID(ctx, nseURL))

	regClient := registry.NewNetworkServiceRegistryClient(nsmClient)

	nseReg, err := regClient.RegisterNSE(context.Background(), &registry.NSERegistration{
		NetworkService: &registry.NetworkService{
			Name: "my-service",
		},
		NetworkServiceEndpoint: &registry.NetworkServiceEndpoint{
			NetworkServiceName: "my-service",
			Url:                "callback:" + nseURL.Path,
		},
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
	require.Equal(t, 2, len(connection.Path.PathSegments))
}
