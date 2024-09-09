// Copyright (c) 2020 Doc.ai and/or its affiliates.
//
// Copyright (c) 2024 Cisco and/or its affiliates.
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
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTempFolder(t *testing.T) {
	folder := TempFolder()
	require.NotNil(t, folder)

	defer func() {
		require.NoError(t, os.Remove(folder))
	}()

	info, err := os.Stat(folder)
	require.False(t, os.IsNotExist(err))
	require.True(t, info.IsDir())
}
