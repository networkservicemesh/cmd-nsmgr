// Copyright (c) 2020-2022 Cisco and/or its affiliates.
//
// Copyright (c) 2021-2022 Doc.ai and/or its affiliates.
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
	"github.com/networkservicemesh/sdk/pkg/tools/debug"
	"github.com/networkservicemesh/sdk/pkg/tools/log"
	"github.com/networkservicemesh/sdk/pkg/tools/log/logruslogger"
	"github.com/networkservicemesh/sdk/pkg/tools/log/spanlogger"
	"github.com/networkservicemesh/sdk/pkg/tools/opentelemetry"
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

	log.EnableTracing(true)
	_, sLogger, span, sFinish := spanlogger.FromContext(ctx, "cmd-nsmgr", map[string]interface{}{})
	defer sFinish()
	_, lLogger, lFinish := logruslogger.FromSpan(ctx, span, "cmd-nsmgr", map[string]interface{}{})
	defer lFinish()
	logger := log.Combine(sLogger, lLogger)

	// ********************************************************************************
	// Debug self if necessary
	// ********************************************************************************
	if err := debug.Self(); err != nil {
		logger.Infof("%s", err)
	}

	// Get cfg from environment
	cfg := &config.Config{}
	if err := envconfig.Usage("nsm", cfg); err != nil {
		logger.Fatal(err)
	}
	if err := envconfig.Process("nsm", cfg); err != nil {
		logger.Fatalf("error processing cfg from env: %+v", err)
	}

	logger.Infof("Using configuration: %v", cfg)

	level, err := logrus.ParseLevel(cfg.LogLevel)
	if err != nil {
		logger.Fatalf("invalid log level %s", cfg.LogLevel)
	}
	logrus.SetLevel(level)
	sFinish()

	// Configure Open Telemetry
	if opentelemetry.IsEnabled() {
		collectorAddress := cfg.OpenTelemetryEndpoint
		spanExporter := opentelemetry.InitSpanExporter(ctx, collectorAddress)
		metricExporter := opentelemetry.InitMetricExporter(ctx, collectorAddress)
		o := opentelemetry.Init(ctx, spanExporter, metricExporter, cfg.Name)
		defer func() {
			if err = o.Close(); err != nil {
				logger.Error(err.Error())
			}
		}()
	}

	logger.Info("MY_INFO")
	err = manager.RunNsmgr(ctx, cfg)
	if err != nil {
		logger.Fatalf("error executing rootCmd: %v", err)
	}
}
