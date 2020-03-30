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

// Package cmd - cobra commands for forwarder
package cmd

import (
	"github.com/spf13/cobra"

	"github.com/networkservicemesh/cmd-forwarder-vppagent/src/forwarder/internal/pkg/flags"
)

func init() {
	flags.CobraCmdDefaults(rootCmd)
}

var rootCmd = &cobra.Command{
	Use:   "forwarder",
	Short: "Provides xconnect network service",
	Long: `Provides xconnect network service.  Supported mechanisms:
     - memif
     - kernel
     - vxlan`,
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Usage()
	},
}

// Execute - execute the command
func Execute() error {
	return rootCmd.Execute()
}
