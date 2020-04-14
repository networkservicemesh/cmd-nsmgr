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
package constants

const (
	// A number of devices we have in buffer for use, so we hold extra DeviceBuffer count of deviceids send to kubelet.
	DeviceBuffer = 30
	//TODO - look at moving the BaseDir to constants somewhere in SDK
	SpireSocket = "/run/spire/sockets"

	// NsmServerSocketEnv is the name of the env variable to define NSM server socket
	NsmServerSocketEnv = "NSM_SERVER_SOCKET"
	// NsmClientSocketEnv is the name of the env variable to define NSM client socket
	NsmClientSocketEnv = "NSM_CLIENT_SOCKET"

	NsmServerSocket = "nsm.server.io.sock"
	NsmClientSocket = "nsm.client.io.sock"

	KubeletServerSock = "networkservicemesh.io.sock"

	// ResourceName - is a Device API resource name
	ResourceName = "networkservicemesh.io/socket"
)
