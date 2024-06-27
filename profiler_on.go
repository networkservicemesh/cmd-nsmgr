// Copyright (c) 2024 Pragmagic Inc. and/or its affiliates.
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

//go:build profiler

package main

import (
	"context"
	"fmt"
	"net/http"
	_ "net/http/pprof" // #nosec
	"time"

	"github.com/networkservicemesh/sdk/pkg/tools/log"
)

func startProfiler(ctx context.Context) {
	go func() {
		profilerHTTPPort := 6060
		log.FromContext(ctx).Infof("Profiler is enabled. Starting HTTP server on %d", profilerHTTPPort)

		address := fmt.Sprintf("localhost:%d", profilerHTTPPort)

		server := &http.Server{
			Addr:         address,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
		}

		if err := server.ListenAndServe(); err != nil {
			log.FromContext(ctx).Errorf("Failed to start profiler: %s", err.Error())
		}
	}()
}
