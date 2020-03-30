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

package flags

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/networkservicemesh/cmd-nsmgr/src/nsmgr/internal/pkg/values"
)

const (
	envPrefix = "NSM"
)

var BaseDir string
var SpiffeAgentURL url.URL
var RegistryURL url.URL

// CobraCmdDefaults - default flags for use in many different commands
func CobraCmdDefaults(cmd *cobra.Command) {
	ViperFlags(cmd.Flags())

	cobra.OnInitialize(func() {
		presetRequiredFlags(cmd)
		postInitCommands(cmd.Commands())
	})
}

func Flags(flags *pflag.FlagSet) {
	// BaseDir
	flags.StringVarP(&BaseDir, "base-dir", "b", "./",
		"BaseDir to use for the memif mechanism")

	// SpiffeAgent To URL
	spiffeAgentURLValue := values.NewGrpcURLValue(&SpiffeAgentURL)
	_ = spiffeAgentURLValue.Set("unix:///run/spire/sockets/agent.sock")
	flags.VarP(spiffeAgentURLValue, "spire-agent-url", "s",
		"URL to Spiffe Agent")

	// Registry URL
	// SpiffeAgent To URL
	registryURLValue := values.NewGrpcURLValue(&RegistryURL)
	_ = registryURLValue.Set("unix:///run/spire/sockets/agent.sock")
	flags.VarP(registryURLValue, "registry-url", "r",
		"URL to Registry")
}

func ViperFlags(flags *pflag.FlagSet) {
	Flags(flags)
	_ = viper.BindPFlags(flags)
	viper.AutomaticEnv()
	viper.SetEnvPrefix(envPrefix)
	viper.SetEnvKeyReplacer(envReplacer())
}

func postInitCommands(commands []*cobra.Command) {
	for _, cmd := range commands {
		presetRequiredFlags(cmd)
		if cmd.HasSubCommands() {
			postInitCommands(cmd.Commands())
		}
	}
}

func presetRequiredFlags(cmd *cobra.Command) {
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if viper.IsSet(f.Name) && viper.GetString(f.Name) != "" {
			err := cmd.Flags().Set(f.Name, viper.GetString(f.Name))
			if err != nil {
				// If root command has SilentErrors flagged,
				// all subcommands should respect it
				if !cmd.SilenceErrors && !cmd.Root().SilenceErrors {
					cmd.Println("Error:", err.Error())
				}

				// If root command has SilentUsage flagged,
				// all subcommands should respect it
				if !cmd.SilenceUsage && !cmd.Root().SilenceUsage {
					cmd.Println(cmd.UsageString())
				}
			}
		}
	})
}

func envReplacer() *strings.Replacer {
	return strings.NewReplacer("-", "_")
}

// KeyToEnvVariable - translate a cobra flag key to a viper ENV variable
func KeyToEnvVariable(key string) string {
	key = strings.ToUpper(key)
	key = envReplacer().Replace(key)
	return fmt.Sprintf("%s_%s", envPrefix, key)
}
