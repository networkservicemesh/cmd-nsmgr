// Copyright (c) 2020 Doc.ai and/or its affiliates.
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

package test

import (
	"fmt"
	"math/rand"
	"net"
	"time"

	"github.com/networkservicemesh/api/pkg/api/registry"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/peer"
)

func (f *NsmgrTestSuite) TestNSMgrRegister() {
	t := f.T()
	setup := newSetup(t)
	rand.Seed(time.Now().Unix())
	setup.configuration.Name = fmt.Sprintf("nsm-%v", rand.Uint64())
	setup.Start()
	defer setup.Stop()

	select {
	case nsmReg := <-setup.registryServer.GetNSMChannel():
		require.NotNil(t, nsmReg)
		logrus.Infof("Registration received %v", nsmReg)
		require.Equal(t, "ready", nsmReg.State)
		require.Equal(t, setup.configuration.Name, nsmReg.Name)
	case <-time.After(10 * time.Second):
		require.Failf(t, "timeout waiting for NSM registration", "")
	}
}

func (f *NsmgrTestSuite) TestNSMgrEndpointRegister() {
	t := f.T()
	setup := newSetup(t)
	setup.Start()
	defer setup.Stop()

	regCtx := peer.NewContext(setup.ctx,
		&peer.Peer{
			Addr: &net.UnixAddr{
				Name: "/var/run/nse-1.sock",
				Net:  "unix",
			},
			AuthInfo: nil,
		})

	regClient := setup.NewRegistryClient(regCtx)

	regRespose, err := regClient.RegisterNSE(regCtx, &registry.NSERegistration{
		NetworkService: &registry.NetworkService{
			Name: "my-network-service",
		},
		NetworkServiceEndpoint: &registry.NetworkServiceEndpoint{
			Name: "my-nse",
		},
	})
	require.Nil(t, err)
	require.NotNil(t, regRespose)
	require.Equal(t, setup.configuration.Name, regRespose.NetworkServiceManager.Name)
	require.Equal(t, "ready", regRespose.NetworkServiceManager.State)
}
