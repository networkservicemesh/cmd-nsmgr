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

	"github.com/networkservicemesh/sdk/pkg/tools/log/logruslogger"
	"github.com/networkservicemesh/sdk/pkg/tools/log/spanlogger"

	"github.com/edwarnicke/grpcfd"
	"github.com/sirupsen/logrus"
	"github.com/spiffe/go-spiffe/v2/spiffetls/tlsconfig"
	"github.com/spiffe/go-spiffe/v2/svid/x509svid"
	"github.com/spiffe/go-spiffe/v2/workloadapi"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/networkservicemesh/sdk/pkg/networkservice/chains/nsmgr"
	"github.com/networkservicemesh/sdk/pkg/networkservice/common/authorize"
	"github.com/networkservicemesh/sdk/pkg/tools/grpcutils"
	"github.com/networkservicemesh/sdk/pkg/tools/log"
	"github.com/networkservicemesh/sdk/pkg/tools/spiffejwt"
	"github.com/networkservicemesh/sdk/pkg/tools/token"
	"github.com/networkservicemesh/sdk/pkg/tools/tracing"

	"github.com/networkservicemesh/cmd-nsmgr/internal/config"
)

const (
	tcpSchema = "tcp"
)

type manager struct {
	ctx           context.Context
	logger        log.Logger
	configuration *config.Config
	cancelFunc    context.CancelFunc
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

	_, sLogger, span, sFinish := spanlogger.FromContext(ctx, "cmd-nsmgr", map[string]interface{}{})
	defer sFinish()
	_, lLogger, lFinish := logruslogger.FromSpan(ctx, span, "cmd-nsmgr", map[string]interface{}{})
	defer lFinish()
	logger := log.Combine(sLogger, lLogger)

	m := &manager{
		configuration: configuration,
		logger:        logger,
	}

	// Context to use for all things started in main
	m.ctx, m.cancelFunc = context.WithCancel(ctx)

	if err := m.initSecurity(); err != nil {
		m.logger.Errorf("failed to create new spiffe TLS Peer %v", err)
		return err
	}

	mgrOptions := []nsmgr.Option{
		nsmgr.WithName(configuration.Name),
		nsmgr.WithURL(m.getPublicURL()),
		nsmgr.WithAuthorizeServer(authorize.NewServer()),
		nsmgr.WithDialTimeout(configuration.DialTimeout),
		nsmgr.WithForwarderServiceName(configuration.ForwarderNetworkServiceName),
		nsmgr.WithDialOptions(
			append(tracing.WithTracingDial(),
				grpc.WithTransportCredentials(
					GrpcfdTransportCredentials(
						credentials.NewTLS(tlsconfig.MTLSClientConfig(m.source, m.source, tlsconfig.AuthorizeAny())),
					),
				),
				grpc.WithBlock(),
				grpc.WithDefaultCallOptions(
					grpc.PerRPCCredentials(token.NewPerRPCCredentials(spiffejwt.TokenGeneratorFunc(m.source, configuration.MaxTokenLifetime))),
				),
				grpcfd.WithChainStreamInterceptor(),
				grpcfd.WithChainUnaryInterceptor(),
			)...,
		),
	}

	if configuration.RegistryURL.String() != "" {
		mgrOptions = append(mgrOptions, nsmgr.WithRegistry(&configuration.RegistryURL, append(
			tracing.WithTracingDial(),
			grpc.WithTransportCredentials(
				GrpcfdTransportCredentials(
					credentials.NewTLS(tlsconfig.MTLSClientConfig(m.source, m.source, tlsconfig.AuthorizeAny())),
				),
			),
			grpc.WithDefaultCallOptions(
				grpc.WaitForReady(true),
			),
		)...))
	}

	m.mgr = nsmgr.NewServer(m.ctx, spiffejwt.TokenGeneratorFunc(m.source, m.configuration.MaxTokenLifetime), mgrOptions...)

	// If we Listen on Unix socket for local connections we need to be sure folder are exist
	createListenFolders(configuration)

	serverOptions := append(
		tracing.WithTracing(),
		grpc.Creds(
			GrpcfdTransportCredentials(
				credentials.NewTLS(tlsconfig.MTLSServerConfig(m.source, m.source, tlsconfig.AuthorizeAny())),
			),
		),
	)

	m.server = grpc.NewServer(serverOptions...)
	m.mgr.Register(m.server)

	// Create GRPC server
	m.startServers(m.server)

	m.logger.Infof("Startup completed in %v", time.Since(starttime))
	starttime = time.Now()
	<-m.ctx.Done()

	m.logger.Infof("Exit requested. Uptime: %v", time.Since(starttime))
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
		m.logger.Warnf("failed to serve: %v", err)
	}
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
		m.logger.Warn(err.Error())
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
			m.logger.Infof("NSMGR Listening on: %v", listenURL.String())
			// For public schemas we need to perform registation of nsmgr into registry.
			wg.Done()

			waitErrChan(m.ctx, errChan, m)
		}()
	}
	wg.Wait()
}
