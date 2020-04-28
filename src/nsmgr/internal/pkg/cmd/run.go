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

	"github.com/networkservicemesh/sdk/pkg/tools/jaeger"
	"github.com/networkservicemesh/sdk/pkg/tools/spanhelper"
	"github.com/opentracing/opentracing-go"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(runCmd)
	CobraCmdDefaults(runCmd)
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Runs Network Service Manager",
	Long: `Runs Network Service Manager.  Supported mechanisms:
     - memif
     - kernel
     - vxlan`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var span opentracing.Span
		if jaeger.IsOpentracingEnabled() {
			jaegerCloser := jaeger.InitJaeger("nsmgr")
			defer func() { _ = jaegerCloser.Close() }()
			span = opentracing.StartSpan("nsmgr")
		}
		cmdSpan := spanhelper.NewSpanHelper(context.Background(), span, "nsmgr-start")
		defer cmdSpan.Finish()
		return RunNsmgr(cmdSpan.Context(), Defaults)
	},
}
