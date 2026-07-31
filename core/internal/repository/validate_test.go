/*
Copyright © contributors to CloudNativePG, established as
CloudNativePG a Series of LF Projects, LLC.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

SPDX-License-Identifier: Apache-2.0
*/

package repository

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsWALFileName(t *testing.T) {
	tests := []struct {
		walName string
		err     error
	}{
		{
			walName: "00000001000000760000007B",
			err:     nil,
		},
		{
			walName: "00000002.history",
			err:     nil,
		},
		{
			walName: "0000000100001234000055CD.007C9330.backup",
			err:     nil,
		},
		{
			walName: "00000001000000760000007BA",
			err:     NewIncorrectWALNameError("00000001000000760000007BA"),
		},
		{
			walName: "00000002.hostory",
			err:     NewIncorrectWALNameError("00000002.hostory"),
		},
		{
			walName: "0000001000000000000001F",
			err:     NewIncorrectWALNameError("0000001000000000000001F"),
		},
	}

	for _, c := range tests {
		t.Run(c.walName, func(t *testing.T) {
			assert.Equal(t, c.err, ValidateWalFileName(c.walName))
		})
	}
}
