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

package main

import (
	"context"
	"os"

	nested "github.com/antonfisher/nested-logrus-formatter"
	"github.com/kelseyhightower/envconfig"
	"github.com/networkservicemesh/cmd-nsmgr/internal/config"
	"github.com/networkservicemesh/cmd-nsmgr/internal/manager"
	"github.com/networkservicemesh/sdk/pkg/tools/jaeger"
	"github.com/networkservicemesh/sdk/pkg/tools/log"
	"github.com/networkservicemesh/sdk/pkg/tools/signalctx"
	"github.com/networkservicemesh/sdk/pkg/tools/spanhelper"
	"github.com/opentracing/opentracing-go"
	"github.com/sirupsen/logrus"
)

func main() {
	// Setup context to catch signals
	// Setup logging
	logrus.SetFormatter(&nested.Formatter{})
	logrus.SetLevel(logrus.TraceLevel)
	ctx := log.WithField(signalctx.WithSignals(context.Background()), "cmd", os.Args[:2])

	var span opentracing.Span
	// Enable Jaeger
	if jaeger.IsOpentracingEnabled() {
		jaegerCloser := jaeger.InitJaeger("nsmgr")
		defer func() { _ = jaegerCloser.Close() }()
		span = opentracing.StartSpan("nsmgr")
	}
	cmdSpan := spanhelper.NewSpanHelper(ctx, span, "nsmgr")

	// Get cfg from environment
	cfg := &config.Config{}
	if err := envconfig.Usage("nsm", cfg); err != nil {
		logrus.Fatal(err)
	}
	if err := envconfig.Process("nsm", cfg); err != nil {
		logrus.Fatalf("error processing cfg from env: %+v", err)
	}

	// Startup is finished
	cmdSpan.Finish()

	err := manager.RunNsmgr(cmdSpan.Context(), cfg)
	if err != nil {
		logrus.Fatalf("error executing rootCmd: %v", err)
	}
}
