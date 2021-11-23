// Copyright (c) 2021 Doc.ai and/or its affiliates.
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

// Package utils contains useful tools
package utils

import (
	"context"

	"github.com/networkservicemesh/sdk/pkg/tools/log"
	"github.com/networkservicemesh/sdk/pkg/tools/log/logruslogger"
	"github.com/networkservicemesh/sdk/pkg/tools/log/spanlogger"
)

// GetTraceLogger creates grouped logger with span and logrus loggers
func GetTraceLogger(ctx context.Context, operation string) (logger log.Logger, cancelFunc func()) {
	ctx, sLogger, span, sFinish := spanlogger.FromContext(ctx, operation)
	ctx, lLogger, lFinish := logruslogger.FromSpan(ctx, span, operation)
	ctxWithLog := log.WithLog(ctx, sLogger, lLogger)
	return log.FromContext(ctxWithLog), func() {
		sFinish()
		lFinish()
	}
}
