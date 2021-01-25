// Copyright (c) 2020-2021 Doc.ai and/or its affiliates.
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
	"fmt"
	"net"
	"net/url"
	"os"
	"path"
	"sync"
	"time"

	"github.com/networkservicemesh/sdk/pkg/tools/logger/tracelogger"

	"github.com/networkservicemesh/sdk/pkg/networkservice/chains/nsmgr"

	"github.com/networkservicemesh/cmd-nsmgr/internal/authz"

	"github.com/networkservicemesh/sdk/pkg/networkservice/common/authorize"
	"github.com/networkservicemesh/sdk/pkg/tools/callback"
	"github.com/networkservicemesh/sdk/pkg/tools/grpcutils"

	"github.com/sirupsen/logrus"
	"github.com/spiffe/go-spiffe/v2/spiffetls/tlsconfig"
	"github.com/spiffe/go-spiffe/v2/svid/x509svid"
	"github.com/spiffe/go-spiffe/v2/workloadapi"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/networkservicemesh/api/pkg/api/registry"

	"github.com/networkservicemesh/sdk/pkg/tools/logger"
	"github.com/networkservicemesh/sdk/pkg/tools/logger/logruslogger"
	"github.com/networkservicemesh/sdk/pkg/tools/opentracing"
	"github.com/networkservicemesh/sdk/pkg/tools/spiffejwt"

	"github.com/networkservicemesh/cmd-nsmgr/internal/config"
)

const (
	tcpSchema = "tcp"
)

type manager struct {
	ctx           context.Context
	configuration *config.Config
	cancelFunc    context.CancelFunc
	registryCC    *grpc.ClientConn
	mgr           nsmgr.Nsmgr
	source        *workloadapi.X509Source
	svid          *x509svid.SVID
	server        *grpc.Server
}

func (m *manager) Stop() {
	m.cancelFunc()
	m.server.Stop()
	_ = m.source.Close()
}

func (m *manager) initSecurity() (err error) {
	// Get a X509Source
	logrus.Infof("Obtaining X509 Certificate Source")
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
		configuration: configuration,
	}
	logger.EnableTracing(true)
	traceCtx, finish := tracelogger.WithLog(ctx, "start")
	defer finish()

	// Context to use for all things started in main
	m.ctx, m.cancelFunc = context.WithCancel(ctx)
	m.ctx, _ = logruslogger.New(
		logger.WithFields(ctx, map[string]interface{}{"cmd": "Nsmgr"}),
	)

	if err := m.initSecurity(); err != nil {
		logger.Log(traceCtx).Errorf("failed to create new spiffe TLS Peer %v", err)
		return err
	}

	if err := m.connectRegistry(); err != nil {
		logger.Log(traceCtx).Errorf("failed to connect registry %v", err)
		return err
	}

	nsmMgr := &registry.NetworkServiceEndpoint{
		Name: configuration.Name,
		Url:  m.getPublicURL(),
	}

	// Construct callback server
	callbackServer := callback.NewServer(authz.IdentityByEndpointID)

	// Construct NSMgr chain
	var regConn grpc.ClientConnInterface
	if m.registryCC != nil {
		regConn = m.registryCC
	}

	clientOptions := append(
		opentracing.WithTracingDial(),
		callbackServer.WithCallbackDialer(),
		// Default client security call options
		grpc.WithTransportCredentials(
			GrpcfdTransportCredentials(
				credentials.NewTLS(tlsconfig.MTLSClientConfig(m.source, m.source, tlsconfig.AuthorizeAny())),
			),
		),
		grpc.WithDefaultCallOptions(grpc.WaitForReady(true)),
	)
	m.mgr = nsmgr.NewServer(m.ctx,
		nsmMgr,
		authorize.NewServer(),
		spiffejwt.TokenGeneratorFunc(m.source, m.configuration.MaxTokenLifetime),
		regConn,
		clientOptions...,
	)

	// If we Listen on Unix socket for local connections we need to be sure folder are exist
	createListenFolders(configuration)

	serverOptions := append(
		opentracing.WithTracing(),
		grpc.Creds(
			GrpcfdTransportCredentials(
				credentials.NewTLS(tlsconfig.MTLSServerConfig(m.source, m.source, tlsconfig.AuthorizeAny())),
			),
		),
	)
	m.server = grpc.NewServer(serverOptions...)
	m.mgr.Register(m.server)

	// Register callback serve to grpc.
	callback.RegisterCallbackServiceServer(m.server, callbackServer)

	// Create GRPC server
	m.startServers(m.server)

	logger.Log(m.ctx).Infof("Startup completed in %v", time.Since(starttime))
	starttime = time.Now()
	<-m.ctx.Done()

	logger.Log(m.ctx).Infof("Exit requested. Uptime: %v", time.Since(starttime))
	// If we here we need to call Stop
	m.Stop()
	return nil
}

func createListenFolders(configuration *config.Config) {
	for i := 0; i < len(configuration.ListenOn); i++ {
		u := &configuration.ListenOn[i]
		if u.Scheme == "unix" {
			nsmDir, _ := path.Split(u.Path)
			_ = os.MkdirAll(nsmDir, os.ModeDir|os.ModePerm)
		}
	}
}

func waitErrChan(ctx context.Context, errChan <-chan error, m *manager) {
	select {
	case <-ctx.Done():
	case err := <-errChan:
		// We need to cal cancel global context, since it could be multiple context of this kind
		m.cancelFunc()
		logger.Log(ctx).Warnf("failed to serve: %v", err)
	}
}

func (m *manager) connectRegistry() (err error) {
	if m.configuration.RegistryURL.String() == "" {
		logrus.Infof("NSM: No NSM registry passed, use memory registry")
		m.registryCC = nil
		return nil
	}
	traceCtx, finish := tracelogger.WithLog(m.ctx, "dial-registry")
	defer finish()

	creds := grpc.WithTransportCredentials(GrpcfdTransportCredentials(credentials.NewTLS(tlsconfig.MTLSClientConfig(m.source, m.source, tlsconfig.AuthorizeAny()))))
	ctx, cancel := context.WithTimeout(traceCtx, 5*time.Second)
	defer cancel()

	logrus.Infof("NSM: Connecting to NSE registry %v", m.configuration.RegistryURL.String())
	options := append(opentracing.WithTracingDial(), creds, grpc.WithDefaultCallOptions(grpc.WaitForReady(true)))
	m.registryCC, err = grpc.DialContext(ctx, grpcutils.URLToTarget(&m.configuration.RegistryURL), options...)
	if err != nil {
		logger.Log(traceCtx).Errorf("failed to dial NSE NsmgrRegistry: %v", err)
	}
	return
}

func (m *manager) defaultURL() *url.URL {
	for i := 0; i < len(m.configuration.ListenOn); i++ {
		u := &m.configuration.ListenOn[i]
		if u.Scheme == tcpSchema {
			return u
		}
	}
	return &m.configuration.ListenOn[0]
}

func (m *manager) getPublicURL() string {
	u := m.defaultURL()
	if u.Port() == "" || len(u.Host) != len(":")+len(u.Port()) {
		return u.String()
	}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		logrus.Warn(err.Error())
		return u.String()
	}
	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return fmt.Sprintf("%v://%v:%v", tcpSchema, ipnet.IP.String(), u.Port())
			}
		}
	}
	return u.String()
}

func (m *manager) startServers(server *grpc.Server) {
	var wg sync.WaitGroup
	for i := 0; i < len(m.configuration.ListenOn); i++ {
		listenURL := &m.configuration.ListenOn[i]
		wg.Add(1)

		go func() {
			// Create a required number of servers
			errChan := grpcutils.ListenAndServe(m.ctx, listenURL, server)
			logrus.Infof("NSMGR Listening on: %v", listenURL.String())
			// For public schemas we need to perform registation of nsmgr into registry.
			wg.Done()

			waitErrChan(m.ctx, errChan, m)
		}()
	}
	wg.Wait()
}
