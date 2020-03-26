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
	"net"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/networkservicemesh/api/pkg/api/networkservice"
	"github.com/networkservicemesh/api/pkg/api/registry"
	"github.com/networkservicemesh/sdk/pkg/tools/security"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"

	"github.com/networkservicemesh/cmd-nsmgr/pkg/nseregistry"
	"github.com/networkservicemesh/cmd-nsmgr/pkg/nsmgr"
)

const (
	// ServerSock defines the path of NSM client socket
	serverSock = "/var/lib/networkservicemesh/nsm.io.sock"
)

func Execute() error {
	rootCmd := &cobra.Command{
		Use:   "nsmgr",
		Short: "Network Service Manager",
		Long:  "Provides Network Service Manager functionality",
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Usage()
		},
	}
	rootCmd.AddCommand(&cobra.Command{
		Use: "run",
		Run: runNSMgr,
	})

	return nil
}

func runNSMgr(cmd *cobra.Command, args []string) {
	// Capture signals to cleanup before exiting - note: this *must* be the first thing in main
	c := make(chan os.Signal, 1)
	signal.Notify(c,
		os.Interrupt,
		// More Linux signals here
		syscall.SIGHUP,
		syscall.SIGTERM,
		syscall.SIGQUIT)

	starttime := time.Now()
	// Setup logging
	logrus.SetReportCaller(true)

	// Context to use for all things started in main
	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()

	// Create listener
	listenOnURL, _ := url.Parse(serverSock)
	ln, err := net.Listen(listenOnURL.Scheme, listenOnURL.Host)
	if err != nil {
		logrus.Fatalf("failed to listen: %v", err)
	}

	// Create GRPC Server
	grpcServer := grpc.NewServer(security.WithSpire(ctx))

	nseRgistryServer := nseregistry.NewServer()
	registry.RegisterNetworkServiceRegistryServer(grpcServer, nseRgistryServer)

	nsmgrServer := nsmgr.NewServer(&nseRgistryServer)
	networkservice.RegisterNetworkServiceServer(grpcServer, nsmgrServer)

	// Serve
	go func() {
		err = grpcServer.Serve(ln)
		if err != nil {
			logrus.Fatalf("failed to serve on %v", listenOnURL)
		}
	}()
	logrus.Infof("Startup completed in %v", time.Since(starttime))

	// Wait for signals
	signal := <-c
	logrus.Infof("Caught signal %+v, exiting...", signal)
	cancelFunc()
}
