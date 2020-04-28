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

// Package authz has convenience functions for getting authz policy
package authz

import (
	"context"
	"io/ioutil"

	"github.com/open-policy-agent/opa/rego"

	"github.com/networkservicemesh/sdk/pkg/tools/log"
)

const (
	// AuthzRegoFilename - default file containing authz rego policy
	AuthzRegoFilename = "/etc/nsm/authz.rego"
	// DefaultAuthzRegoContents - default authz rego policy
	DefaultAuthzRegoContents = `package test

default allow = true
`
)

// PolicyFromFile - either reads in the policy file or uses the default values
func PolicyFromFile(ctx context.Context, filename, defaultIfNotFound string) (rego.PreparedEvalQuery, error) {
	// OpenPolicyAgent Authz Policy
	policyBytes, err := ioutil.ReadFile(filename)
	if err != nil {
		log.Entry(ctx).Infof("Unable to open Authz policy file %q,falling back to default", filename)
		policyBytes = []byte(defaultIfNotFound)
	}
	policy := string(policyBytes)
	return rego.New(
		// TODO - rework the example.com and test pieces here
		rego.Query("data.test.allow"),
		rego.Module("example.com", policy)).PrepareForEval(ctx)
}
