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

package tier2

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCredentialsFileProviderReadsRotatedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials")
	write := func(content string) {
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}
	write("[default]\naws_access_key_id = old\naws_secret_access_key = oldsecret\n" +
		"[klio]\naws_access_key_id = k1\naws_secret_access_key = s1\naws_session_token = t1\n")

	def := credentialsFileProvider{path: path}
	creds, err := def.Retrieve(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "old", creds.AccessKeyID)
	assert.True(t, creds.CanExpire)
	assert.False(t, creds.Expires.IsZero())

	named := credentialsFileProvider{path: path, profile: "klio"}
	creds, err = named.Retrieve(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "k1", creds.AccessKeyID)
	assert.Equal(t, "t1", creds.SessionToken)

	write("[klio]\naws_access_key_id = k2\naws_secret_access_key = s2\n")
	creds, err = named.Retrieve(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "k2", creds.AccessKeyID)
	assert.Equal(t, "s2", creds.SecretAccessKey)
}

func TestCredentialsFileProviderErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials")
	require.NoError(t, os.WriteFile(path, []byte("[default]\nregion = x\n"), 0o600))

	_, err := credentialsFileProvider{path: path}.Retrieve(t.Context())
	require.ErrorIs(t, err, ErrCredentialsFileNoKeys)

	_, err = credentialsFileProvider{path: filepath.Join(t.TempDir(), "missing")}.Retrieve(t.Context())
	require.Error(t, err)
}
