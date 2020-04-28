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
	"bufio"
	"context"
	"fmt"
	"github.com/networkservicemesh/cmd-nsmgr/src/nsm/internal/pkg/tools"
	"github.com/networkservicemesh/sdk/pkg/tools/debug"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"os"
	"path"
	"strings"

	"github.com/networkservicemesh/sdk/pkg/tools/executils"
	"github.com/networkservicemesh/sdk/pkg/tools/log"
)

var listArguments = struct {
	spire       bool
	cgo_enabled bool
}{}

func init() {
	cmd := listCmd
	rootCmd.AddCommand(cmd)

	listCmd.Flags().BoolVarP(&testArguments.cgoEnabled,
		"cgo", "", false, "If disabled will pass CGO_ENABLED=0 env variable to go compiler (default disabled)")

	listCmd.Flags().BoolVarP(&testArguments.spire,
		"spire", "s", true, "If enabled will run spire (default enabled)")
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "Perform a list of available tests",
	Long:  `Perform a list of available tests`,
	RunE: func(cmd *cobra.Command, args []string) error {
		logrus.Infof("NSM.List target...")
		if err := debug.Self(); err != nil {
			log.Entry(cmd.Context()).Infof("%s", err)
		}

		isDocker := tools.IsDocker()

		_, cgoEnv := tools.RetrieveGoEnv(cmdArguments.cgoEnabled, cmdArguments.goos, cmdArguments.goarch)

		if len(args) == 0 && !isDocker {
			//We look for packages only in docker env, else we run only /bin/*.test applications.
			args = tools.FindMainPackages(cmd.Context(), cgoEnv)
		}

		// Final All test packages
		packages := map[string]map[string]*tools.PackageInfo{}
		for _, rootDir := range args {
			// We in state to run tests,
			pkgs, err := tools.FindTests(cmd.Context(), rootDir, cgoEnv)
			if err != nil {
				logrus.Errorf("failed to find tests %v", err)
			}
			// Add spire entries for every appliction and test application we found.
			_, cmdName := path.Split(path.Clean(rootDir))
			packages[cmdName] = pkgs
		}
		for _, testApp := range packages {
			for _, testPkg := range testApp {
				if len(testPkg.Tests) > 0 {
					// Print test info
					testExecName := path.Join("/bin", testPkg.OutName)
					logrus.Infof("Test binary: %v tests: %v", testExecName, testPkg.Tests)
				}
			}
		}
		return nil
	},
}

func buildTarget(ctx context.Context, target string) (containerId string, err error) {
	logrus.Infof("Build target %v with docker...", target)

	reader, writer, err := os.Pipe()
	if err != nil {
		logrus.Errorf("failed to create pipe: %v", err)
		return "", err
	}
	go func() {
		err := executils.Run(ctx, fmt.Sprintf("docker build --build-arg BUILD=false . --target %s", target),
			executils.WithStdout(writer),
			executils.WithStderr(writer))
		if err != nil {
			logrus.Errorf("Failed to run docker build %v", err)
		}
		_ = reader.Close()
		_ = writer.Close()
	}()

	sr := bufio.NewReader(reader)
	lastLine := ""
	for {
		str, err := sr.ReadString('\n')
		if err != nil {
			break
		}
		if len(str) > 0 {
			lastLine = str
		}
		logrus.Infof("Docker build: %v", str)
	}
	prefix := "Successfully built "
	if strings.HasPrefix(lastLine, prefix) {
		containerId = strings.TrimSpace(lastLine[len(prefix):len(lastLine)])
	} else {
		err = errors.Errorf("Failed to parse container id %v", lastLine)
		return
	}
	return
}
