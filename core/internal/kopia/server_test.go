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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const fingerprintFlag = "--server-cert-fingerprint="

// newArgsRecordingKopia writes a fake kopia binary that appends its
// arguments, one per line, to the returned file.
func newArgsRecordingKopia(t *testing.T, dir string) (string, string) {
	t.Helper()

	argsFile := filepath.Join(dir, "args")
	binary := filepath.Join(dir, "kopia")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" >> '" + argsFile + "'\n"
	require.NoError(t, os.WriteFile(binary, []byte(script), 0o700)) //nolint:gosec // test executable

	return binary, argsFile
}

func recordedFingerprints(t *testing.T, argsFile string) []string {
	t.Helper()

	content, err := os.ReadFile(argsFile) //nolint:gosec // test file
	require.NoError(t, err)

	var fingerprints []string
	for line := range strings.SplitSeq(string(content), "\n") {
		if fingerprint, ok := strings.CutPrefix(line, fingerprintFlag); ok {
			fingerprints = append(fingerprints, fingerprint)
		}
	}

	return fingerprints
}

func TestRefreshServerFollowsRotatedCertificate(t *testing.T) {
	dir := t.TempDir()
	binary, argsFile := newArgsRecordingKopia(t, dir)
	certPath := writeFingerprintTestCert(t, dir, "kopia-server")

	client := &Client{KopiaBinary: binary}
	opts := RefreshServerOptions{
		ServerCertFile: certPath,
		Address:        "https://localhost:51515",
	}

	require.NoError(t, client.RefreshServer(t.Context(), opts))

	// A renewal rewrites the file in place: the next refresh must pin the
	// certificate the server now presents, not the one seen at startup.
	_ = writeFingerprintTestCert(t, dir, "kopia-server")
	require.NoError(t, client.RefreshServer(t.Context(), opts))

	current, err := LeafFingerprint(certPath)
	require.NoError(t, err)

	fingerprints := recordedFingerprints(t, argsFile)
	require.Len(t, fingerprints, 2)
	require.NotEqual(t, fingerprints[0], fingerprints[1])
	require.Equal(t, current, fingerprints[1])
}

func TestRefreshServerMissingCertificate(t *testing.T) {
	dir := t.TempDir()
	binary, argsFile := newArgsRecordingKopia(t, dir)

	client := &Client{KopiaBinary: binary}
	err := client.RefreshServer(t.Context(), RefreshServerOptions{
		ServerCertFile: filepath.Join(dir, "absent.crt"),
		Address:        "https://localhost:51515",
	})
	require.ErrorIs(t, err, os.ErrNotExist)
	require.NoFileExists(t, argsFile)
}
