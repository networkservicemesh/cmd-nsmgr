// Copyright (c) 2020 Cisco and/or its affiliates.
//
// Copyright (c) 2021 Doc.ai and/or its affiliates.
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
	"github.com/google/uuid"
	"github.com/kelseyhightower/envconfig"
	"github.com/sirupsen/logrus"

	"github.com/networkservicemesh/sdk/pkg/tools/debug"
	"github.com/networkservicemesh/sdk/pkg/tools/jaeger"
	"github.com/networkservicemesh/sdk/pkg/tools/log"
	"github.com/networkservicemesh/sdk/pkg/tools/log/logruslogger"
	"github.com/networkservicemesh/sdk/pkg/tools/log/spanlogger"
	"github.com/networkservicemesh/sdk/pkg/tools/signalctx"

	"github.com/networkservicemesh/cmd-nsmgr/internal/config"
	"github.com/networkservicemesh/cmd-nsmgr/internal/manager"
)

func main() {
	println(uuid.New().String())

	// Setup conmomod text to catch signals
	// Setup logging
	ctx := signalctx.WithSignals(context.Background())
	logrus.SetFormatter(&nested.Formatter{})
	ctx = log.WithFields(ctx, map[string]interface{}{"cmd": os.Args[:1]})
	ctx = log.WithLog(ctx, logruslogger.New(ctx))

	// ********************************************************************************
	// Debug self if necessary
	// ********************************************************************************
	if err := debug.Self(); err != nil {
		log.FromContext(ctx).Infof("%s", err)
	}

	// ********************************************************************************
	// Configure open tracing
	// ********************************************************************************
	// Enable Jaeger
	log.EnableTracing(true)
	jaegerCloser := jaeger.InitJaeger(ctx, "nsmgr")
	defer func() { _ = jaegerCloser.Close() }()
	traceCtx, finish := withTraceLogger(ctx, "nsmgr")

	// Get cfg from environment
	cfg := &config.Config{}
	if err := envconfig.Usage("nsm", cfg); err != nil {
		logrus.Fatal(err)
	}
	if err := envconfig.Process("nsm", cfg); err != nil {
		logrus.Fatalf("error processing cfg from env: %+v", err)
	}

	logrus.Infof("Using configuration: %v", cfg)

	// Startup is finished
	finish()

	err := manager.RunNsmgr(traceCtx, cfg)
	if err != nil {
		logrus.Fatalf("error executing rootCmd: %v", err)
	}
}

func withTraceLogger(ctx context.Context, operation string) (c context.Context, f func()) {
	ctx, sLogger, span, sFinish := spanlogger.FromContext(ctx, operation)
	ctx, lLogger, lFinish := logruslogger.FromSpan(ctx, span, operation)
	return log.WithLog(ctx, sLogger, lLogger), func() {
		sFinish()
		lFinish()
	}
}
