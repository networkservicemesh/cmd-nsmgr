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
	"context"
	"net"
	"net/url"

	"github.com/networkservicemesh/api/pkg/api/networkservice"
	"github.com/networkservicemesh/sdk/pkg/networkservice/chains/nsmgr"
	"github.com/networkservicemesh/sdk/pkg/networkservice/chains/nsmgr/peertracker"
	"github.com/networkservicemesh/sdk/pkg/tools/serialize"
	"google.golang.org/grpc"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

type NsmDevicePluginServer interface {
	nsmgr.Nsmgr
	pluginapi.DevicePluginServer
}

type nsmgrDevicePlugin struct {
	nsmgr.Nsmgr
	devices               map[string]*pluginapi.Device
	allocatedDevices      map[string]*pluginapi.Device
	executor              serialize.Executor
	insecure              bool
	reallocate            func(u *url.URL)
	listAndWatchListeners []pluginapi.DevicePlugin_ListAndWatchServer
}

func NewServer(name string, insecure bool, registryCC *grpc.ClientConn) NsmDevicePluginServer {
	rv := &nsmgrDevicePlugin{
		devices:          make(map[string]*pluginapi.Device, DeviceBuffer),
		allocatedDevices: make(map[string]*pluginapi.Device, DeviceBuffer),
		executor:         serialize.NewExecutor(),
		insecure:         insecure,
	}
	// TODO - Fix applying peer_tracker here
	// rv.Nsmgr = peer_tracker.NewServer(nsmgr.NewEndpoint(name, registryCC), &rv.reallocate)
	rv.Nsmgr = peertracker.NewServer(nsmgr.NewNsmgr(name, registryCC), &rv.reallocate)
	rv.resizeDevicePool()
	return rv
}

func (n *nsmgrDevicePlugin) ListAndWatch(e *pluginapi.Empty, s pluginapi.DevicePlugin_ListAndWatchServer) error {
	n.executor.AsyncExec(func() {
		n.listAndWatchListeners = append(n.listAndWatchListeners, s)
		listAndWatchResponse := &pluginapi.ListAndWatchResponse{}
		for _, device := range n.devices {
			listAndWatchResponse.Devices = append(listAndWatchResponse.Devices, device)
		}
		for _, listAndWatchListener := range n.listAndWatchListeners {
			listAndWatchListener.Send(listAndWatchResponse)
		}
	})

	<-s.Context().Done()
	n.executor.AsyncExec(func() {
		var listAndWatchListeners []pluginapi.DevicePlugin_ListAndWatchServer
		for _, listAndWatchListener := range n.listAndWatchListeners {
			if listAndWatchListener != s {
				listAndWatchListeners = append(listAndWatchListeners, listAndWatchListener)
			}
		}
	})
	return nil
}

func (n *nsmgrDevicePlugin) Allocate(ctx context.Context, reqs *pluginapi.AllocateRequest) (*pluginapi.AllocateResponse, error) {
	containerResponses := []*pluginapi.ContainerAllocateResponse{}
	for _, req := range reqs.GetContainerRequests() {
		for _, deviceid := range req.GetDevicesIDs() {
			// Close any existing connection from previous allocation
			n.reallocate(&url.URL{
				Scheme: "unix",
				Path:   localServerSocketFile(deviceid),
			})
			mounts := []*pluginapi.Mount{
				{
					ContainerPath: containerDeviceDirectory(deviceid),
					HostPath:      hostDeviceDirectory(deviceid),
					ReadOnly:      false,
				},
			}
			envs := map[string]string{
				NsmServerSocketEnv: containerServerSocketFile(deviceid),
				NsmClientSocketEnv: containerClientSocketFile(deviceid),
			}
			if !n.insecure {
				mounts = append(mounts, &pluginapi.Mount{
					ContainerPath: SpireSocket,
					HostPath:      SpireSocket,
					ReadOnly:      true,
				})
			}
			containerResponse := &pluginapi.ContainerAllocateResponse{
				Envs:   envs,
				Mounts: mounts,
			}
			containerResponses = append(containerResponses, containerResponse)
			n.executor.AsyncExec(func() {
				n.allocatedDevices[deviceid] = n.devices[deviceid]
			})
		}
	}
	n.resizeDevicePool()
	return &pluginapi.AllocateResponse{ContainerResponses: containerResponses}, nil
}

func (n *nsmgrDevicePlugin) PreStartContainer(context.Context, *pluginapi.PreStartContainerRequest) (*pluginapi.PreStartContainerResponse, error) {
	return &pluginapi.PreStartContainerResponse{}, nil
}

func (n *nsmgrDevicePlugin) GetDevicePluginOptions(context.Context, *pluginapi.Empty) (*pluginapi.DevicePluginOptions, error) {
	return &pluginapi.DevicePluginOptions{}, nil
}

func (n *nsmgrDevicePlugin) resizeDevicePool() {
	n.executor.AsyncExec(func() {
		for len(n.devices)-len(n.allocatedDevices) < DeviceBuffer {
			device := &pluginapi.Device{
				ID:     "nsm-" + string(len(n.devices)),
				Health: pluginapi.Healthy,
			}
			listener, err := net.Listen("unix", localServerSocketFile(device.GetID()))
			if err != nil {
				// Note: There's nothing productive we can do about this other than failing here
				// and thus not increasing the device pool
				return
			}
			grpcServer := grpc.NewServer()
			go func() {
				grpcServer.Serve(listener)
			}()
			n.Nsmgr.Register(grpcServer)
			n.devices[device.GetID()] = device
		}
		listAndWatchResponse := &pluginapi.ListAndWatchResponse{}
		for _, device := range n.devices {
			listAndWatchResponse.Devices = append(listAndWatchResponse.Devices, device)
		}
		for _, listAndWatchListener := range n.listAndWatchListeners {
			listAndWatchListener.Send(listAndWatchResponse)
		}
	})
}
