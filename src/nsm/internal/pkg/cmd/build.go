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
	"fmt"
	"github.com/networkservicemesh/cmd-nsmgr/src/nsm/internal/pkg/tools"
	"github.com/networkservicemesh/sdk/pkg/tools/debug"
	"github.com/networkservicemesh/sdk/pkg/tools/executils"
	"github.com/networkservicemesh/sdk/pkg/tools/log"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"path"
	"sync"
)

type BuildCmdArguments struct {
	outputFolder string
	compileTests bool

	goarch     string
	goos       string
	cgoEnabled bool
	docker     bool
}

var cmdArguments = &BuildCmdArguments{}

func init() {
	rootCmd.AddCommand(buildCmd)

	buildCmd.Flags().StringVarP(&cmdArguments.outputFolder,
		"output", "o", "./dist", "Output folder (default ./dist)")

	buildCmd.Flags().BoolVarP(&cmdArguments.compileTests,
		"tests", "t", true, "Compile individual test packages")

	buildCmd.Flags().BoolVarP(&cmdArguments.docker,
		"docker", "", true, "If enabled, will do docker build . --build-arg BUILD=false after local build will be done")

	buildCmd.Flags().BoolVarP(&cmdArguments.cgoEnabled,
		"cgo", "", false, "If disabled will pass CGO_ENABLED=0 env variable to go compiler (default disabled)")

	buildCmd.Flags().StringVarP(&cmdArguments.goos,
		"goos", "", "linux", "If passed will pass GOOS=${value} env variable (default linux)")

	buildCmd.Flags().StringVarP(&cmdArguments.goarch,
		"goarch", "", "amd64", "If passed will pass GOARCH=${value} env variable (default amd64)")
}

var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Perform a build of passed applications and tests CGO_ENABLED=0 GOOS=linux GOARCH=amd64",
	Long:  "Perform a build of passed application and all tests related to it with CGO_ENABLED=0 GOOS=linux GOARCH=amd64",
	RunE: func(cmd *cobra.Command, args []string) error {
		logrus.Infof("NSM.Build target...")
		if err := debug.Self(); err != nil {
			log.Entry(cmd.Context()).Infof("%s", err)
		}

		err := PerformBuild(cmd, args, cmdArguments)
		if err != nil {
			logrus.Infof("Build complete")
		}
		return err
	},
}

func PerformBuild(cmd *cobra.Command, args []string, cmdArguments *BuildCmdArguments) error {
	env, cgoEnv := tools.RetrieveGoEnv(cmdArguments.cgoEnabled, cmdArguments.goos, cmdArguments.goarch)

	if len(args) == 0 {
		args = tools.FindMainPackages(cmd.Context(), cgoEnv)
	}

	var wg sync.WaitGroup
	var pkgError error
	for _, root := range args {
		rootDir := path.Clean(root)
		_, cmdName := path.Split(path.Clean(rootDir))

		wg.Add(1)
		go func() {
			defer wg.Done()
			logrus.Infof("Building: %v at %v", cmdName, rootDir)
			buildCmd := fmt.Sprintf("go build -o %v %v", path.Join(cmdArguments.outputFolder, cmdName), "./"+rootDir)
			if err := executils.Run(cmd.Context(), buildCmd, executils.WithEnviron(env)); err != nil {
				logrus.Errorf("Error build: %v %v", buildCmd, err)
				pkgError = err
			}
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			testPackages, err := tools.FindTests(cmd.Context(), rootDir, cgoEnv)
			if err != nil {
				pkgError = err
				return
			}
			for k, p := range testPackages {
				if len(p.Tests) > 0 {
					pp := p
					logrus.Infof("Found tests: %v for package: %v", pp.Tests, k)
					if cmdArguments.compileTests {
						wg.Add(1)
						go func() {
							defer wg.Done()
							testPath := "./" + path.Join(rootDir, pp.RelPath)
							buildCmd := fmt.Sprintf("go test -c -o %v %v", path.Join(cmdArguments.outputFolder, pp.OutName), testPath)
							if err := executils.Run(cmd.Context(), buildCmd, executils.WithEnviron(env)); err != nil {
								logrus.Errorf("Error build: %v %v", buildCmd, err)
								pkgError = err
								return
							}
							logrus.Infof("Compile of %v from %v complete", pp.OutName, testPath)
						}()
					}
				}
			}
		}()
	}
	wg.Wait()
	if pkgError != nil {
		logrus.Errorf("Build failed %v", pkgError)
		return pkgError
	}

	if cmdArguments.docker && !tools.IsDocker() {
		logrus.Infof("Building docker container")
		err := executils.Run(cmd.Context(), fmt.Sprintf("docker build . --build-arg BUILD=false"))
		if err != nil {
			logrus.Errorf("Failed to build docker container %v", err)
			return err
		}
	}
	return nil
}
