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
	"context"
	"github.com/networkservicemesh/cmd-nsmgr/src/nsmgr/internal/pkg/flags"
	"github.com/networkservicemesh/sdk/pkg/tools/jaeger"
	"github.com/networkservicemesh/sdk/pkg/tools/spanhelper"
	"github.com/opentracing/opentracing-go"
	"github.com/spf13/cobra"
	"io"
)

var cmdContext context.Context
var cmdCancel context.CancelFunc

var cmdSpan spanhelper.SpanHelper

func init() {
	flags.CobraCmdDefaults(rootCmd)
	resetContext(context.Background())
}

func resetContext(ctx context.Context) {
	// Cancel previous command
	if cmdCancel != nil {
		cmdCancel()
	}
	cmdContext, cmdCancel = context.WithCancel(ctx)
}

var jaegerCloser io.Closer

var rootCmd = &cobra.Command{
	Use:   "nsmgr",
	Short: "Provides Network Service Manager",
	Long:  `Provides Network Service Manager.`,
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Usage()
	},
	PreRun: func(cmd *cobra.Command, args []string) {
		var span opentracing.Span
		if jaeger.IsOpentracingEnabled() {
			jaegerCloser = jaeger.InitJaeger("nsmgr")
			span = opentracing.StartSpan("nsmgr")
		}
		cmdSpan = spanhelper.NewSpanHelper(cmdContext, span, "nsmgr-start")
	},
	PostRun: func(cmd *cobra.Command, args []string) {
		if jaegerCloser != nil {
			_ = jaegerCloser.Close()
		}
		cmdSpan.Finish()
	},
}

// Execute - execute the command
func Execute() error {
	return rootCmd.Execute()
}

// TestExecute - execute the command with test parameters
func TestExecute(ctx context.Context, cmdLine ...string) error {
	resetContext(ctx)
	rootCmd.SetArgs(cmdLine)
	return rootCmd.Execute()
}
