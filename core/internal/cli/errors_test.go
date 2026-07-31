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

package cli

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCodedErrorExitCode(t *testing.T) {
	err := NewCodedError(errors.New("boom"), 1)
	var coded ExitCoder
	require.ErrorAs(t, err, &coded)
	assert.Equal(t, 1, coded.ExitCode())
}

func TestNewCodedErrorNil(t *testing.T) {
	assert.NoError(t, NewCodedError(nil, 1))
}

func TestCodedErrorUnwrap(t *testing.T) {
	inner := errors.New("inner")
	wrapped := NewCodedError(inner, 1)
	require.ErrorIs(t, wrapped, inner)
	assert.Equal(t, "inner", wrapped.Error())
}

func TestCodedErrorErrorsAs(t *testing.T) {
	inner := errors.New("inner")
	wrapped := fmt.Errorf("outer: %w", NewCodedError(inner, 1))

	var coded ExitCoder
	require.ErrorAs(t, wrapped, &coded)
	assert.Equal(t, 1, coded.ExitCode())
}
