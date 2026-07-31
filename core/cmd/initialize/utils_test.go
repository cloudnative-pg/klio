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

package initialize

import (
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCanInitRepoDirectory(t *testing.T) {
	memFs := afero.NewMemMapFs()
	_ = afero.WriteFile(memFs, "/nonEmptyDirectory/test.txt", []byte("test"), 0o600)
	_ = memFs.Mkdir("/emptyDirectory", 0o777)

	v, err := canInitRepoDirectory(afero.NewBasePathFs(memFs, "/nonEmptyDirectory"))
	require.NoError(t, err)
	assert.False(t, v, "should not be able to init existing non-empty directory")

	v, err = canInitRepoDirectory(afero.NewBasePathFs(memFs, "/emptyDirectory"))
	require.NoError(t, err)
	assert.True(t, v, "should be able to init existing empty directory")

	v, err = canInitRepoDirectory(afero.NewBasePathFs(memFs, "/nonExistingDirectory"))
	require.NoError(t, err)
	assert.True(t, v, "should be able to init non existing directory")

	// error should be returned because the path is not a directory
	v, err = canInitRepoDirectory(afero.NewBasePathFs(memFs, "/nonEmptyDirectory/test.txt"))
	require.Error(t, err)
	assert.False(t, v, "should not be able to init when cannot read directory contents")
}
