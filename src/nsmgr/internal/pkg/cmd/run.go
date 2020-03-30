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

package cmd

import (
	"context"
	"net"

	"os"
	"os/signal"
	"path"
	"syscall"
	"time"

	"github.com/networkservicemesh/sdk/pkg/tools/grpcoptions"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"


	"github.com/networkservicemesh/cmd-nsmgr/src/nsmgr/internal/pkg/deviceplugin"
	"github.com/networkservicemesh/cmd-nsmgr/src/nsmgr/internal/pkg/flags"
)

const (
	ServerSock            = "networkservicemesh.io.sock"
)

func init() {
	rootCmd.AddCommand(runCmd)
	flags.CobraCmdDefaults(runCmd)
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Runs xconnect network service",
	Long: `Runs xconnect network service.  Supported mechanisms:
     - memif
     - kernel
     - vxlan`,
	Run: func(cmd *cobra.Command, args []string) {
		// Capture signals to cleanup before exiting - note: this *must* be the first thing in main
		c := make(chan os.Signal, 1)
		signal.Notify(c,
			os.Interrupt,
			// More Linux signals here
			syscall.SIGHUP,
			syscall.SIGTERM,
			syscall.SIGQUIT)

		// Setup logging
		logrus.SetReportCaller(true)

		// Context to use for all things started in main
		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()

		registryCC, err := grpc.DialContext(ctx, flags.RegistryURL.String(), grpcoptions.WithSpiffe(&flags.SpiffeAgentURL, 10*time.Millisecond))

		dp := deviceplugin.NewServer(viper.GetString("name"), viper.GetBool("insecure"), registryCC)
		listenEndpoint := path.Join(pluginapi.DevicePluginPath, ServerSock)
		sock, err := net.Listen("unix", listenEndpoint)
		if err != nil {
			logrus.Fatalf("failed to listen on %s: %+v", listenEndpoint, err)
		}
		grpcServer := grpc.NewServer()
		pluginapi.RegisterDevicePluginServer(grpcServer, dp)
		go func() {
			if err := grpcServer.Serve(sock); err != nil {
				logrus.Error("failed to start device plugin grpc server", listenEndpoint, err)
			}
		}()
	},
}
