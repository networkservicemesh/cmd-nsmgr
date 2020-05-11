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

// Package callbacknse define a test NSE listening on passed URL.
package callbacknse

import (
	"context"
	"net/url"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/networkservicemesh/api/pkg/api/networkservice"
	"github.com/networkservicemesh/cmd-nsmgr/internal/grpcutils"
	"google.golang.org/grpc"
)

type nseImpl struct {
	server    *grpc.Server
	ctx       context.Context
	cancel    context.CancelFunc
	listenOn  *url.URL
	errorChan <-chan error
	update    func(request *networkservice.NetworkServiceRequest)
}

// NewNSE construct a new NSE with callback up and running on grpc server, listenOn is updated if :0 port is passed.
func NewNSE(ctx context.Context, listenOn *url.URL, update func(request *networkservice.NetworkServiceRequest)) (server *grpc.Server, errChan <-chan error) {
	nse := &nseImpl{
		listenOn: listenOn,
		server:   grpc.NewServer(),
		update:   update,
	}
	networkservice.RegisterNetworkServiceServer(nse.server, nse)

	nse.ctx, nse.cancel = context.WithCancel(ctx)
	nse.errorChan = grpcutils.ListenAndServe(nse.ctx, nse.listenOn, nse.server)
	return nse.server, nse.errorChan
}

func (d *nseImpl) Request(ctx context.Context, request *networkservice.NetworkServiceRequest) (*networkservice.Connection, error) {
	request.Connection.Labels = map[string]string{}
	if d.update != nil {
		d.update(request)
	}
	return request.GetConnection(), nil
}

func (d *nseImpl) Close(ctx context.Context, connection *networkservice.Connection) (*empty.Empty, error) {
	return &empty.Empty{}, nil
}
