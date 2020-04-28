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

package cmd

import (
	"context"
	"github.com/networkservicemesh/api/pkg/api"
	"github.com/networkservicemesh/cmd-nsmgr/src/nsmgr/internal/pkg/authz"
	"github.com/networkservicemesh/sdk/pkg/tools/callback"
	"github.com/networkservicemesh/sdk/pkg/tools/errctx"
	"github.com/networkservicemesh/sdk/pkg/tools/grpcutils"
	"github.com/networkservicemesh/sdk/pkg/tools/log"
	"github.com/networkservicemesh/sdk/pkg/tools/signalctx"
	"github.com/networkservicemesh/sdk/pkg/tools/spiffeutils"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"os"
	"path"
	"time"

	"github.com/networkservicemesh/sdk/pkg/networkservice/chains/nsmgr"
	"github.com/networkservicemesh/sdk/pkg/tools/spanhelper"
	"github.com/sirupsen/logrus"
	"github.com/spiffe/go-spiffe/spiffe"
	"google.golang.org/grpc"
)

func RunNsmgr(ctx context.Context, values *DefinedFlags) error {
	starttime := time.Now()

	span := spanhelper.FromContext(ctx, "run")
	defer span.Finish()
	ctx = span.Context()

	ctx = signalctx.WithSignals(ctx)

	var err error

	// Setup logging
	logrus.SetReportCaller(true)

	// Context to use for all things started in main
	var cancelFunc context.CancelFunc
	ctx, cancelFunc = context.WithCancel(ctx)
	defer cancelFunc()

	var spiffieTLSPeer spiffeutils.TLSPeer
	spiffieTLSPeer, err = spiffeutils.NewTLSPeer(spiffe.WithWorkloadAPIAddr(values.SpiffeAgentURL.String()))
	if err != nil {
		span.LogErrorf("failed to create new spiffe TLS Peer %v", err)
		return err
	}

	registryCC, err2 := newRegistry(ctx, values, spiffieTLSPeer)
	if err2 != nil {
		return err2
	}

	// Get OpenPolicyAgent Authz Policy
	authzPolicy, err := authz.PolicyFromFile(ctx, authz.AuthzRegoFilename, authz.DefaultAuthzRegoContents)
	if err != nil {
		log.Entry(ctx).Fatalf("Unable to open Authz policy file %q", authz.AuthzRegoFilename)
	}

	// Dummy provider by identity, TODO: Make a proper identification of clients.
	callbackServer := callback.NewServer(callback.IdentityByAuthority)

	mgr := nsmgr.NewServer(values.Name, &authzPolicy, nil, registryCC)

	// Listen on Unix socket for local connections
	nsmDir, _ := path.Split(values.ListenOnURL.Path)
	_ = os.MkdirAll(nsmDir, os.ModeDir|os.ModePerm)

	// Create GRPC server
	server := grpc.NewServer(spiffeutils.SpiffeCreds(spiffieTLSPeer, 15*time.Second))
	mgr.Register(server)

	// Register callback server
	callback.RegisterCallbackServiceServer(server, callbackServer)

	// Create GRPC Health Server:
	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(server, healthServer)
	for _, service := range api.ServiceNames(mgr) {
		healthServer.SetServingStatus(service, grpc_health_v1.HealthCheckResponse_SERVING)
	}

	srvCtx := grpcutils.ListenAndServe(ctx, &values.ListenOnURL, server)

	log.Entry(ctx).Infof("Startup completed in %v", time.Since(starttime))
	select {
	case <-ctx.Done():
	case <-srvCtx.Done():
		log.Entry(srvCtx).Warnf("failed to serve on %q: %+v", &values.ListenOnURL, errctx.Err(srvCtx))
		//os.Exit(1)
	}

	return nil
}

func newRegistry(ctx context.Context, values *DefinedFlags, spiffieTLSPeer spiffeutils.TLSPeer) (grpc.ClientConnInterface, error) {
	regSpan := spanhelper.FromContext(ctx, "dial-registry")
	defer regSpan.Finish()
	registryCC, err := grpc.DialContext(regSpan.Context(), values.RegistryURL.String(), spiffeutils.WithSpiffe(spiffieTLSPeer, 15*time.Second), grpc.WithBlock())

	if err != nil {
		regSpan.LogErrorf("failed to dial NSE Registry: %v", err)
		return nil, err
	}
	regSpan.Finish()
	return registryCC, err
}
