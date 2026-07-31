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

package kopia

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseVerifyOutput(t *testing.T) {
	t.Run("valid JSON with errors", func(t *testing.T) {
		stdout := []byte(`{"errorCount":2,"errorStrings":["bad object 1","bad object 2"]}`)
		result := parseVerifyOutput(stdout)
		assert.Equal(t, 2, result.ErrorCount)
		assert.Equal(t, []string{"bad object 1", "bad object 2"}, result.ErrorStrings)
	})

	t.Run("valid JSON with no errors", func(t *testing.T) {
		stdout := []byte(`{"errorCount":0}`)
		result := parseVerifyOutput(stdout)
		assert.Equal(t, 0, result.ErrorCount)
		assert.Empty(t, result.ErrorStrings)
	})

	t.Run("empty output returns zero result", func(t *testing.T) {
		result := parseVerifyOutput([]byte{})
		assert.Equal(t, VerifyResult{}, result)
	})

	t.Run("invalid JSON returns zero result", func(t *testing.T) {
		result := parseVerifyOutput([]byte("not json"))
		assert.Equal(t, VerifyResult{}, result)
	})
}
