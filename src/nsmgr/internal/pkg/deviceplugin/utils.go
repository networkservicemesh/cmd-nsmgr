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

package deviceplugin

import (
	"github.com/networkservicemesh/cmd-nsmgr/src/nsmgr/internal/pkg/constants"
	"github.com/networkservicemesh/cmd-nsmgr/src/nsmgr/internal/pkg/flags"
)

// A container endpoint and memif directory.
func containerDeviceDirectory(deviceId string) string {
	return flags.Values.BaseDir + "/client/" + deviceId
}

// A container server directory.
func containerServerDirectory(deviceId string) string {
	return flags.Values.BaseDir + "/nsm/"
}

// A host device directory
func hostServerDirectory(deviceId string) string {
	return flags.Values.BaseDir + "/nsm/"
}

// A host device directory
func hostDeviceDirectory(deviceId string) string {
	return flags.Values.BaseDir + "/" + deviceId + "/"
}

// Container server socket file
func containerServerSocketFile(deviceId string) string {
	return containerServerDirectory(deviceId) + constants.NsmServerSocket
}

func hostServerSocketFile(deviceId string) string {
	return containerServerDirectory(deviceId) + constants.NsmServerSocket
}

func containerClientSocketFile(deviceId string) string {
	return containerDeviceDirectory(deviceId) + constants.NsmClientSocket
}
