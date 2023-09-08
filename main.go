// Copyright (c) 2020-2023 Cisco and/or its affiliates.
//
// Copyright (c) 2021-2023 Doc.ai and/or its affiliates.
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
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/kelseyhightower/envconfig"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/trace"

	"github.com/networkservicemesh/cmd-nsmgr/internal/config"
	"github.com/networkservicemesh/cmd-nsmgr/internal/manager"
	"github.com/networkservicemesh/sdk/pkg/tools/debug"
	"github.com/networkservicemesh/sdk/pkg/tools/log"
	"github.com/networkservicemesh/sdk/pkg/tools/log/logruslogger"
	"github.com/networkservicemesh/sdk/pkg/tools/opentelemetry"
)

func main() {
	// Setup context to catch signals
	ctx, cancel := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		// More Linux signals here
		syscall.SIGHUP,
		syscall.SIGTERM,
		syscall.SIGQUIT,
	)
	defer cancel()

	// Setup logging
	ctx = log.WithLog(ctx, logruslogger.New(ctx, map[string]interface{}{"cmd": os.Args[0]}))

	// ********************************************************************************
	// Debug self if necessary
	// ********************************************************************************
	if err := debug.Self(); err != nil {
		log.FromContext(ctx).Infof("%s", err)
	}

	// Get cfg from environment
	cfg := &config.Config{}
	if err := envconfig.Usage("nsm", cfg); err != nil {
		log.FromContext(ctx).Fatal(err)
	}
	if err := envconfig.Process("nsm", cfg); err != nil {
		log.FromContext(ctx).Fatalf("error processing cfg from env: %+v", err)
	}

	log.FromContext(ctx).Infof("Using configuration: %v", cfg)

	level, err := logrus.ParseLevel(cfg.LogLevel)
	if err != nil {
		log.FromContext(ctx).Fatalf("invalid log level %s", cfg.LogLevel)
	}
	logrus.SetLevel(level)
	log.EnableTracing(level == logrus.TraceLevel)

	// Configure Open Telemetry
	if opentelemetry.IsEnabled() {
		metricExporter := strings.ToLower(cfg.MetricsExporter)
		collectorAddress := cfg.OpenTelemetryEndpoint
		var spanExporter trace.SpanExporter
		var metricReader metric.Reader

		if collectorAddress != "" {
			spanExporter = opentelemetry.InitSpanExporter(ctx, collectorAddress)
		}

		var o io.Closer

		switch metricExporter {
		case "prometheus":
			metricReader = opentelemetry.InitPrometheusMetricExporter(ctx)
		default:
			if collectorAddress != "" {
				metricReader = opentelemetry.InitOPTLMetricExporter(ctx, collectorAddress)
			}
		}

		o = opentelemetry.Init(ctx, spanExporter, metricReader, cfg.Name)

		if metricExporter == "prometheus" {
			go serveMetrics(ctx, cfg.MetricsPort)
		}

		defer func() {
			if err = o.Close(); err != nil {
				log.FromContext(ctx).Error(err.Error())
			}
		}()
	}

	err = manager.RunNsmgr(ctx, cfg)
	if err != nil {
		log.FromContext(ctx).Fatalf("error executing rootCmd: %v", err)
	}
}

// https://github.com/open-telemetry/opentelemetry-go/blob/v1.17.0/example/prometheus/main.go
func serveMetrics(ctx context.Context, port int) {
	log.FromContext(ctx).Infof(fmt.Sprintf("serving metrics at localhost:%d/metrics", port))
	http.Handle("/metrics", promhttp.Handler())
	err := http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
	if err != nil {
		fmt.Printf("error serving http: %v", err)
		return
	}
}
