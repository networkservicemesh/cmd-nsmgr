// Copyright (c) 2020-2022 Doc.ai and/or its affiliates.
//
// Copyright (c) 2023-2024 Cisco and/or its affiliates.
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

// Package config - contain environment variables used by nsmgr
package config

import (
	"net/url"
	"time"
)

// Config - configuration for cmd-nsmgr
type Config struct {
	Name                        string        `default:"nmgr" desc:"Name of Network service manager"`
	ListenOn                    []url.URL     `default:"unix:///var/lib/networkservicemesh/nsm.io.sock" desc:"url to listen on. tcp:// one will be used a public to register NSM." split_words:"true"`
	RegistryURL                 url.URL       `default:"tcp://localhost:5001" desc:"A NSE registry url to use" split_words:"true"`
	MaxTokenLifetime            time.Duration `default:"10m" desc:"maximum lifetime of tokens" split_words:"true"`
	RegistryServerPolicies      []string      `default:"etc/nsm/opa/common/.*.rego,etc/nsm/opa/registry/.*.rego,etc/nsm/opa/server/.*.rego" desc:"paths to files and directories that contain registry server policies" split_words:"true"`
	RegistryClientPolicies      []string      `default:"etc/nsm/opa/common/.*.rego,etc/nsm/opa/registry/.*.rego,etc/nsm/opa/client/.*.rego" desc:"paths to files and directories that contain registry client policies" split_words:"true"`
	LogLevel                    string        `default:"INFO" desc:"Log level" split_words:"true"`
	DialTimeout                 time.Duration `default:"750ms" desc:"Timeout for the dial the next endpoint" split_words:"true"`
	ForwarderNetworkServiceName string        `default:"forwarder" desc:"the default service name for forwarder discovering" split_words:"true"`
	OpenTelemetryEndpoint       string        `default:"otel-collector.observability.svc.cluster.local:4317" desc:"OpenTelemetry Collector Endpoint"`
	MetricsExportInterval       time.Duration `default:"10s" desc:"interval between mertics exports" split_words:"true"`
}
