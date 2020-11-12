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
	"io/ioutil"
	"net/url"
	"os"
	"path"
	"runtime"
	"testing"
	"time"

	"github.com/networkservicemesh/sdk/pkg/registry/common/sendfd"
	"github.com/networkservicemesh/sdk/pkg/registry/core/next"
	"github.com/networkservicemesh/sdk/pkg/tools/log"

	"github.com/sirupsen/logrus"

	"github.com/networkservicemesh/sdk/pkg/networkservice/common/clienturl"
	"github.com/networkservicemesh/sdk/pkg/networkservice/common/connect"
	"github.com/networkservicemesh/sdk/pkg/networkservice/core/adapters"
	"github.com/networkservicemesh/sdk/pkg/registry/common/interpose"
	"github.com/networkservicemesh/sdk/pkg/registry/core/chain"
	"github.com/networkservicemesh/sdk/pkg/tools/addressof"
	"github.com/networkservicemesh/sdk/pkg/tools/token"

	"github.com/networkservicemesh/sdk/pkg/networkservice/chains/endpoint"
	"github.com/networkservicemesh/sdk/pkg/networkservice/common/authorize"
	"github.com/networkservicemesh/sdk/pkg/networkservice/common/setextracontext"
	"github.com/networkservicemesh/sdk/pkg/tools/grpcutils"

	"github.com/spiffe/go-spiffe/v2/spiffetls/tlsconfig"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/stretchr/testify/require"

	"github.com/networkservicemesh/api/pkg/api/networkservice"
	"github.com/networkservicemesh/api/pkg/api/networkservice/mechanisms/cls"
	"github.com/networkservicemesh/api/pkg/api/networkservice/mechanisms/kernel"
	"github.com/networkservicemesh/api/pkg/api/registry"
	"github.com/networkservicemesh/cmd-nsmgr/internal/authz"

	"github.com/networkservicemesh/sdk/pkg/networkservice/chains/client"
	"github.com/networkservicemesh/sdk/pkg/tools/callback"
	"github.com/networkservicemesh/sdk/pkg/tools/spiffejwt"
)

func serve(ctx context.Context, listenOn *url.URL, e endpoint.Endpoint, opt ...grpc.ServerOption) (<-chan error, *grpc.Server) {
	server := grpc.NewServer(opt...)
	e.Register(server)

	return grpcutils.ListenAndServe(ctx, listenOn, server), server
}

type myEndpouint struct {
	endpoint.Endpoint
}

// NewCrossNSE construct a new Cross connect test NSE
func newCrossNSE(ctx context.Context, name string, connectTo *url.URL, tokenGenerator token.GeneratorFunc, clientDialOptions ...grpc.DialOption) endpoint.Endpoint {
	var crossNSe = &myEndpouint{}
	crossNSe.Endpoint = endpoint.NewServer(ctx,
		name,
		authorize.NewServer(),
		tokenGenerator,
		// Statically set the url we use to the unix file socket for the NSMgr
		clienturl.NewServer(connectTo),
		connect.NewServer(
			ctx,
			client.NewClientFactory(
				name,
				// What to call onHeal
				addressof.NetworkServiceClient(adapters.NewServerToClient(crossNSe)),
				tokenGenerator,
			),
			clientDialOptions...,
		),
	)
	return crossNSe
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

	ctx = log.WithField(ctx, "chain", "Client")

	nsmClient := setup.newClient(ctx)

	nseURL := &url.URL{Scheme: "tcp", Host: "127.0.0.1:0"}

	nseErr, nseGRPC := serve(ctx, nseURL,
		endpoint.NewServer(ctx, "nse",
			authorize.NewServer(),
			spiffejwt.TokenGeneratorFunc(setup.Source, setup.configuration.MaxTokenLifetime),
			setextracontext.NewServer(map[string]string{"perform": "ok"})),
		grpc.Creds(credentials.NewTLS(tlsconfig.MTLSServerConfig(setup.Source, setup.Source, tlsconfig.AuthorizeAny()))))

	require.NotNil(t, nseErr)
	require.NotNil(t, nseGRPC)

	// Serve callbacks
	callbackClient := callback.NewClient(nsmClient, nseGRPC)
	// Construct context to pass identity to server.
	callbackClient.Serve(authz.WithCallbackEndpointID(ctx, nseURL))

	nsRegClient := registry.NewNetworkServiceRegistryClient(nsmClient)
	regClient := registry.NewNetworkServiceEndpointRegistryClient(nsmClient)
	ns, _ := nsRegClient.Register(ctx, &registry.NetworkService{
		Name: "my-service",
	})

	nseReg, err := regClient.Register(ctx, &registry.NetworkServiceEndpoint{
		NetworkServiceNames: []string{ns.Name},
		Url:                 "callback:" + nseURL.String(),
	})
	require.Nil(t, err)
	require.NotNil(t, nseReg)

	f.registerCrossNSE(ctx, setup, regClient, t)

	cl := client.NewClient(ctx, "nsc-1", nil, spiffejwt.TokenGeneratorFunc(setup.Source, setup.configuration.MaxTokenLifetime), nsmClient)

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
	require.Equal(t, 5, len(connection.Path.PathSegments))

	_, err = cl.Close(ctx, connection)
	require.Nil(t, err)
}

// Check endpoint registration and Client request to it with sendfd/recvfd
func (f *NsmgrTestSuite) TestNSmgrEndpointSendFD() {
	if runtime.GOOS != "linux" {
		f.T().Skip("not a linux")
	}
	t := f.T()
	// TODO: check with defer goleak.VerifyNone(t)
	setup := newSetup(t)
	setup.Start()
	defer setup.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	nsmClient := setup.newClient(ctx)

	rootDir, _ := ioutil.TempDir(os.TempDir(), "nsmgr")

	nseURL := &url.URL{Scheme: "unix", Path: path.Join(rootDir, "endpoint.socket")}

	nseErr, nseGRPC := serve(ctx, nseURL,
		endpoint.NewServer(ctx, "nse", authorize.NewServer(), spiffejwt.TokenGeneratorFunc(setup.Source, setup.configuration.MaxTokenLifetime), setextracontext.NewServer(map[string]string{"perform": "ok"})),
		grpc.Creds(credentials.NewTLS(tlsconfig.MTLSServerConfig(setup.Source, setup.Source, tlsconfig.AuthorizeAny()))))

	require.NotNil(t, nseErr)
	require.NotNil(t, nseGRPC)

	nsRegClient := registry.NewNetworkServiceRegistryClient(nsmClient)
	regClient := next.NewNetworkServiceEndpointRegistryClient(
		sendfd.NewNetworkServiceEndpointRegistryClient(),
		registry.NewNetworkServiceEndpointRegistryClient(nsmClient),
	)
	logrus.Infof("Register network service")
	ns, nserr := nsRegClient.Register(context.Background(), &registry.NetworkService{
		Name: "my-service",
	})

	require.NoError(t, nserr)

	logrus.Infof("Register NSE")

	nseReg, err := regClient.Register(context.Background(), &registry.NetworkServiceEndpoint{
		NetworkServiceNames: []string{ns.Name},
		Url:                 nseURL.String(),
	})
	require.Nil(t, err)
	require.NotNil(t, nseReg)

	logrus.Infof("Register cross NSE")

	f.registerCrossNSE(ctx, setup, regClient, t)

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
	require.Equal(t, 5, len(connection.Path.PathSegments))

	_, err = cl.Close(ctx, connection)
	require.Nil(t, err)
}

func (f *NsmgrTestSuite) registerCrossNSE(ctx context.Context, setup *testSetup, regClient registry.NetworkServiceEndpointRegistryClient, t *testing.T) {
	// Serve Cross Connect NSE
	crossNSEURL := &url.URL{Scheme: "tcp", Host: "127.0.0.1:0"}
	endpoint.Serve(ctx, crossNSEURL,
		newCrossNSE(ctx, "cross-nse", &setup.configuration.ListenOn[0], spiffejwt.TokenGeneratorFunc(setup.Source, setup.configuration.MaxTokenLifetime),
			grpc.WithTransportCredentials(credentials.NewTLS(tlsconfig.MTLSClientConfig(setup.Source, setup.Source, tlsconfig.AuthorizeAny()))),
			grpc.WithDefaultCallOptions(grpc.WaitForReady(true))),
		grpc.Creds(credentials.NewTLS(tlsconfig.MTLSServerConfig(setup.Source, setup.Source, tlsconfig.AuthorizeAny()))))
	logrus.Infof("Cross NSE listenON: %v", crossNSEURL.String())

	// Register Cross NSE
	crossRegClient := chain.NewNetworkServiceEndpointRegistryClient(interpose.NewNetworkServiceEndpointRegistryClient(), regClient)
	_, err := crossRegClient.Register(ctx, &registry.NetworkServiceEndpoint{
		Url:  crossNSEURL.String(),
		Name: "cross-nse",
	})
	require.Nil(t, err)
}
