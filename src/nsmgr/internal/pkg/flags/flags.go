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
	"github.com/networkservicemesh/cmd-nsmgr/src/nsmgr/internal/pkg/constants"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
	"net/url"
	"path"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/networkservicemesh/cmd-nsmgr/src/nsmgr/internal/pkg/values"
)

const (
	envPrefix = "NSM"
)

// DefinedFlags - a set of flag values
type DefinedFlags struct {
	BaseDir string

	SpiffeAgentURL url.URL
	RegistryURL    url.URL

	// Some environment variables
	Insecure bool
	Name     string

	DeviceAPIListenEndpoint string
	DeviceAPIRegistryServer string
	DeviceAPIPluginPath     string
}

// Defaults - default values loaded from environment
var Defaults = &DefinedFlags{
	Insecure:                true,
	Name:                    "Unnamed",
	DeviceAPIListenEndpoint: path.Join(pluginapi.DevicePluginPath, constants.KubeletServerSock),
	DeviceAPIRegistryServer: "unix:" + pluginapi.KubeletSocket,
	DeviceAPIPluginPath:     pluginapi.DevicePluginPath,
}

// DefineFlags - redefine flags
func DefineFlags(useDefault bool, defineF func(flags *DefinedFlags)) {
	val := *Defaults
	if !useDefault {
		val = *Values
	}
	Values = &val
	defineF(Values)
}

// RestoreFlags - restore flags based on defaults
func RestoreFlags() {
	Values = Defaults
}

// Values - a current set valuse, tests could change this
var Values = Defaults

// CobraCmdDefaults - default flags for use in many different commands
func CobraCmdDefaults(cmd *cobra.Command) {
	ViperFlags(cmd.Flags())

	cobra.OnInitialize(func() {
		presetRequiredFlags(cmd)
		postInitCommands(cmd.Commands())
	})
}

func Flags(f *pflag.FlagSet) {
	// BaseDir
	f.StringVarP(&Defaults.BaseDir, "base-dir", "b", "./",
		"BaseDir to hold all sockets and mechanism files.")

	// SpiffeAgent To URL
	spiffeAgentURLValue := values.NewGrpcURLValue(&Defaults.SpiffeAgentURL)
	_ = spiffeAgentURLValue.Set("unix:///run/spire/sockets/agent.sock")
	f.VarP(spiffeAgentURLValue, "spire-agent-url", "s",
		"URL to Spiffe Agent")

	// Registry URL
	registryURLValue := values.NewGrpcURLValue(&Defaults.RegistryURL)
	_ = registryURLValue.Set("unix:///run/networkservicemesh/registry/registry.sock")
	f.VarP(registryURLValue, "registry-url", "r",
		"URL to Registry")
}
func ViperFlags(flags *pflag.FlagSet) {
	Flags(flags)
	_ = viper.BindPFlags(flags)
	viper.AutomaticEnv()
	viper.SetEnvPrefix(envPrefix)
	viper.SetEnvKeyReplacer(envReplacer())

	// Extract some values from viper
	ViperLoadFlags()
}

func ViperLoadFlags() {
	Defaults.Insecure = viper.GetBool("insecure")
	Defaults.Name = viper.GetString("name")
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
