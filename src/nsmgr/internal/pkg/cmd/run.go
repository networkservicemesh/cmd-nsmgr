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
	"github.com/fsnotify/fsnotify"
	"github.com/networkservicemesh/cmd-nsmgr/src/nsmgr/internal/pkg/constants"
	"github.com/networkservicemesh/sdk/pkg/networkservice/chains/nsmgr"
	"github.com/spiffe/go-spiffe/spiffe"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
	"net"
	"path"

	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/networkservicemesh/cmd-nsmgr/src/nsmgr/internal/pkg/deviceplugin"
	"github.com/networkservicemesh/cmd-nsmgr/src/nsmgr/internal/pkg/flags"
	"github.com/networkservicemesh/sdk/pkg/tools/grpcoptions"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
)

func init() {
	rootCmd.AddCommand(runCmd)
	flags.CobraCmdDefaults(runCmd)
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Runs Network Service Manager",
	Long: `Runs Network Service Manager.  Supported mechanisms:
     - memif
     - kernel
     - vxlan`,
	Run: func(cmd *cobra.Command, args []string) {
		var err error
		// Capture signals to cleanup before exiting - note: this *must* be the first thing in main
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs,
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

		var spiffieTlsPeer *spiffe.TLSPeer
		if !flags.Values.Insecure {
			spiffieTlsPeer, err = spiffe.NewTLSPeer(spiffe.WithWorkloadAPIAddr(flags.Values.SpiffeAgentURL.String()))
			if err != nil {
				logrus.Fatalf("failed to create new spiffe TLS Peer %v", err)
			}
		}

		var registryCC grpc.ClientConnInterface
		registryCC, err = grpc.DialContext(ctx, flags.Values.RegistryURL.String(), grpcoptions.WithSpiffe(spiffieTlsPeer, 15*time.Second), grpc.WithBlock())

		if err != nil {
			logrus.Fatalf("failed to dial NSE Registry")
		}
		nsmgr := nsmgr.NewServer(flags.Values.Name, nil, registryCC)

		nsmDir := path.Join(flags.Values.BaseDir, "nsm")
		_ = os.MkdirAll(nsmDir, os.ModeDir|os.ModePerm)
		var listener net.Listener
		listener, err = net.Listen("unix", path.Join(nsmDir, constants.NsmServerSocket))
		if err != nil {
			// Note: There's nothing productive we can do about this other than failing here
			// and thus not increasing the device pool
			return
		}

		grpcServer := grpc.NewServer(grpcoptions.SpiffeCreds(spiffieTlsPeer, 5*time.Second))
		nsmgr.Register(grpcServer)

		go func() {
			_ = grpcServer.Serve(listener)
		}()

		// Start device plugin
		dp := deviceplugin.NewServer(flags.Values.Insecure)

		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			logrus.Fatalf("failed to create FS watcher")
		}

		// Listen for kubelet device api register socket, we need to re-register in case this socket is deleted, created agains.
		err = watcher.Add(flags.Values.DeviceAPIPluginPath)
		if err != nil {
			_ = watcher.Close()
			logrus.Fatalf("failed to create FS watcher")
		}

	restart:
		dp.Stop()
		err = dp.Start()
		if err != nil {
			goto restart
		}
	events:
		for {

			select {
			case event := <-watcher.Events:
				if event.Name == pluginapi.KubeletSocket && event.Op&fsnotify.Create == fsnotify.Create {
					logrus.Printf("inotify: %s created, restarting.", pluginapi.KubeletSocket)
					goto restart
				}

			case ierr := <-watcher.Errors:
				logrus.Printf("inotify: %s", ierr)
			case s := <-sigs:
				switch s {
				case syscall.SIGHUP:
					logrus.Println("Received SIGHUP, restarting.")
					goto restart
				default:
					logrus.Printf("Received signal \"%v\", shutting down.", s)
					dp.Stop()
					break events
				}
			}
		}
	},
}
