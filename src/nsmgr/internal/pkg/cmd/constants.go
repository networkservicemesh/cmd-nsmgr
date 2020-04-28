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

// Package constants - a most of constant definitions.
package cmd

const (
	envPrefix = "NSM"

	// ListenOnURLKey - key for flag for ListenOnURL
	ListenOnTCPURLKey = "listen-on-tcp-url"
	// ListenOnTCPURLPathDefault - default path for ListenOnURL
	ListenOnTCPURLPathDefault = ":5001"
	// ListenOnURLUsageDefault - default usage for ListenOnURL
	ListenOnTCPURLUsageDefault = "TCP URL to listen for incoming networkservicemesh RPC calls"

	NsmServerSocket        = "nsm.server.io.sock"
	ListenOnURLPathDefault = "/var/lib/networkservicemesh/nsm/" + NsmServerSocket
)
