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
	"os/signal"
	"syscall"

	"github.com/kelseyhightower/envconfig"
	"github.com/sirupsen/logrus"

	"github.com/networkservicemesh/cmd-nsmgr/internal/config"
	"github.com/networkservicemesh/cmd-nsmgr/internal/manager"
	"github.com/networkservicemesh/cmd-nsmgr/internal/utils"
	"github.com/networkservicemesh/sdk/pkg/tools/debug"
	"github.com/networkservicemesh/sdk/pkg/tools/jaeger"
	"github.com/networkservicemesh/sdk/pkg/tools/log"
)

func main() {
	// Setup conmomod text to catch signals
	// Setup logging
	ctx, cancel := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		// More Linux signals here
		syscall.SIGHUP,
		syscall.SIGTERM,
		syscall.SIGQUIT,
	)
	defer cancel()

	traceCtx := log.WithFields(ctx, map[string]interface{}{"cmd": os.Args[:1]})
	traceLogger, finish := utils.GetTraceLogger(traceCtx, "nsmgr")
	defer finish()

	// ********************************************************************************
	// Debug self if necessary
	// ********************************************************************************
	if err := debug.Self(); err != nil {
		traceLogger.Infof("%s", err)
	}

	// ********************************************************************************
	// Configure open tracing
	// ********************************************************************************
	// Enable Jaeger
	log.EnableTracing(true)
	jaegerCloser := jaeger.InitJaeger(traceCtx, "nsmgr")
	defer func() { _ = jaegerCloser.Close() }()

	// Get cfg from environment
	cfg := &config.Config{}
	if err := envconfig.Usage("nsm", cfg); err != nil {
		traceLogger.Fatal(err)
	}
	if err := envconfig.Process("nsm", cfg); err != nil {
		traceLogger.Fatalf("error processing cfg from env: %+v", err)
	}

	traceLogger.Infof("Using configuration: %v", cfg)

	level, err := logrus.ParseLevel(cfg.LogLevel)
	if err != nil {
		traceLogger.Fatalf("invalid log level %s", cfg.LogLevel)
	}
	logrus.SetLevel(level)

	err = manager.RunNsmgr(ctx, cfg)
	if err != nil {
		traceLogger.Fatalf("error executing rootCmd: %v", err)
	}
}
