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

// Package test contains tests for nsmgr.
package test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	nested "github.com/antonfisher/nested-logrus-formatter"
	"github.com/sirupsen/logrus"
	"github.com/spiffe/go-spiffe/v2/workloadapi"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/networkservicemesh/sdk/pkg/tools/spire"
)

type NsmgrTestSuite struct {
	suite.Suite

	ctx        context.Context
	cancel     context.CancelFunc
	spireErrCh <-chan error
}

func (f *NsmgrTestSuite) SetupSuite() {
	logrus.SetFormatter(&nested.Formatter{})
	logrus.SetLevel(logrus.TraceLevel)
	f.ctx, f.cancel = context.WithCancel(context.Background())

	// Run spire
	executable, err := os.Executable()
	require.NoError(f.T(), err)

	reuseSpire := os.Getenv(workloadapi.SocketEnv) != ""
	if !reuseSpire {
		f.spireErrCh = spire.Start(
			spire.WithContext(f.ctx),
			spire.WithEntry("spiffe://example.org/nsmgr", "unix:path:/bin/nsmgr"),
			spire.WithEntry("spiffe://example.org/nsmgr.test", "unix:uid:0"),
			spire.WithEntry(fmt.Sprintf("spiffe://example.org/%s", filepath.Base(executable)),
				fmt.Sprintf("unix:path:%s", executable),
			),
		)
	}
}
func (f *NsmgrTestSuite) TearDownSuite() {
	f.cancel()
	if f.spireErrCh != nil {
		for {
			_, ok := <-f.spireErrCh
			if !ok {
				break
			}
		}
	}
}

// In order for 'go test' to run this suite, we need to create
// a normal test function and pass our suite to suite.Run
func TestRegistryTestSuite(t *testing.T) {
	suite.Run(t, new(NsmgrTestSuite))
}
