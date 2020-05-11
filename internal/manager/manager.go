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

// Package manager contains nsmgr main code.
package manager

import (
	"context"
	"os"
	"path"
	"sync"
	"time"

	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/networkservicemesh/api/pkg/api/registry"
	"github.com/networkservicemesh/cmd-nsmgr/internal/chains/nsmgr"
	"github.com/networkservicemesh/cmd-nsmgr/internal/config"
	"github.com/networkservicemesh/cmd-nsmgr/internal/grpcutils"
	"github.com/networkservicemesh/sdk/pkg/tools/log"
	"github.com/networkservicemesh/sdk/pkg/tools/signalctx"
	"github.com/networkservicemesh/sdk/pkg/tools/spanhelper"
	"github.com/networkservicemesh/sdk/pkg/tools/spiffejwt"
	"github.com/sirupsen/logrus"
	"github.com/spiffe/go-spiffe/v2/spiffetls/tlsconfig"
	"github.com/spiffe/go-spiffe/v2/svid/x509svid"
	"github.com/spiffe/go-spiffe/v2/workloadapi"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

const tcpSchema = "tcp"

type manager struct {
	ctx           context.Context
	configuration *config.Config
	span          spanhelper.SpanHelper
	cancelFunc    context.CancelFunc
	registryCC    *grpc.ClientConn
	mgr           nsmgr.Nsmgr
	source        *workloadapi.X509Source
	svid          *x509svid.SVID
}

func (m *manager) Stop() {
	m.cancelFunc()
}

func (m *manager) initSecurity() (err error) {
	// Get a X509Source
	m.source, err = workloadapi.NewX509Source(m.ctx)
	if err != nil {
		logrus.Fatalf("error getting x509 source: %+v", err)
	}
	m.svid, err = m.source.GetX509SVID()
	if err != nil {
		logrus.Fatalf("error getting x509 svid: %+v", err)
	}
	logrus.Infof("SVID: %q", m.svid.ID)
	return
}

// RunNsmgr - start nsmgr.
func RunNsmgr(ctx context.Context, configuration *config.Config) error {
	starttime := time.Now()

	m := &manager{
		ctx:           ctx,
		configuration: configuration,
		span:          spanhelper.FromContext(ctx, "start"),
	}
	defer m.Stop()

	// Update context
	m.ctx = signalctx.WithSignals(m.span.Context())

	// Context to use for all things started in main
	m.ctx, m.cancelFunc = context.WithCancel(ctx)

	if err := m.initSecurity(); err != nil {
		m.span.LogErrorf("failed to create new spiffe TLS Peer %v", err)
		return err
	}

	if err := m.connectRegistry(); err != nil {
		m.span.LogErrorf("failed to connect registry %v", err)
		return err
	}

	nsmMgr := &registry.NetworkServiceManager{
		Name:  configuration.Name,
		State: "ready",
		Url:   configuration.ListenOn[0].String(),
	}

	// Construct NSMgr chain
	m.mgr = nsmgr.NewServer(
		nsmMgr,
		spiffejwt.TokenGeneratorFunc(m.source, m.configuration.MaxTokenLifetime),
		m.registryCC)

	// If we Listen on Unix socket for local connections we need to be sure folder are exist
	createListenFolders(configuration)

	server := grpc.NewServer(grpc.Creds(credentials.NewTLS(tlsconfig.MTLSServerConfig(m.source, m.source, tlsconfig.AuthorizeAny()))))
	m.mgr.Register(server)

	var wg sync.WaitGroup
	// Create GRPC server
	m.startServers(nsmMgr, &wg, server)

	wg.Wait() // Wait for GRPC is ready before registering

	// Register Network service manager.
	wg.Add(1)
	regChan := registerNSM(ctx, m.mgr, nsmMgr, configuration, &wg)
	wg.Wait()

	log.Entry(ctx).Infof("Startup completed in %v", time.Since(starttime))

	// Wait until context is done, or error is received.
	waitErrChan(ctx, regChan, m, configuration)

	return nil
}

func createListenFolders(configuration *config.Config) {
	for _, u := range configuration.ListenOn {
		if u.Scheme == "unix" {
			nsmDir, _ := path.Split(u.Path)
			_ = os.MkdirAll(nsmDir, os.ModeDir|os.ModePerm)
		}
	}
}

func waitErrChan(ctx context.Context, errChan <-chan error, m *manager, configuration *config.Config) {
	select {
	case <-ctx.Done():
	case err := <-errChan:
		// We need to cal cancel global context, since it could be multiple context of this kind
		m.cancelFunc()
		log.Entry(ctx).Warnf("failed to serve on %q: %+v", &configuration.ListenOn, err)
	}
}

// registerNSM - perform a periodic registation of current nsm to update validity interval.
func registerNSM(ctx context.Context, nsmRegistry registry.NsmRegistryServer, nsmMgr *registry.NetworkServiceManager, configuration *config.Config, wg *sync.WaitGroup) <-chan error {
	errChan := make(chan error, 10)
	initComplete := false
	go func() {
		for {
			aliveTime := configuration.RegistrationInterval
			expireTime := time.Now().Add(aliveTime + aliveTime/5) // We add 20% threshold

			// Set expire time to proper value
			nsmMgr.ExpirationTime = &timestamp.Timestamp{
				Seconds: expireTime.Unix(),
			}

			mgr, err := nsmRegistry.RegisterNSM(ctx, nsmMgr)
			if err != nil {
				logrus.Errorf("Failed to register NSM %v", err)
				errChan <- err
				close(errChan)
				wg.Done()
				return
			}
			if !initComplete {
				wg.Done()
				initComplete = true
				logrus.Infof("Registered to registry %v", mgr)
			} else {
				logrus.Infof("Update alive registry %v", mgr)
			}
			time.Sleep(aliveTime)
		}
	}()
	return errChan
}
func (m *manager) connectRegistry() (err error) {
	regSpan := spanhelper.FromContext(m.ctx, "dial-registry")
	defer regSpan.Finish()

	creds := grpc.WithTransportCredentials(credentials.NewTLS(tlsconfig.MTLSClientConfig(m.source, m.source, tlsconfig.AuthorizeAny())))
	ctx, cancel := context.WithTimeout(regSpan.Context(), 5*time.Second)
	defer cancel()

	logrus.Infof("NSM: Connecting to NSE registry %v", m.configuration.RegistryURL.String())
	m.registryCC, err = grpc.DialContext(ctx, grpcutils.URLToTarget(m.configuration.RegistryURL), creds, grpc.WithDefaultCallOptions(grpc.WaitForReady(true)))
	if err != nil {
		regSpan.LogErrorf("failed to dial NSE NsmgrRegistry: %v", err)
	}
	return
}

func (m *manager) startServers(nsmMgr *registry.NetworkServiceManager, wg *sync.WaitGroup, server *grpc.Server) {
	for _, u := range m.configuration.ListenOn {
		listenURL := u
		wg.Add(1)

		// In case of Public IP we need to add +1 to wait before registation will be done.
		if listenURL.Scheme == tcpSchema {
			nsmMgr.Url = listenURL.String()
		}

		go func() {
			// Create a required number of servers
			errChan := grpcutils.ListenAndServe(m.ctx, listenURL, server)
			if listenURL.Scheme == tcpSchema {
				nsmMgr.Url = listenURL.String()
			}
			// For public schemas we need to perform registation of nsmgr into registry.
			wg.Done()

			waitErrChan(m.ctx, errChan, m, m.configuration)
		}()
	}
}
