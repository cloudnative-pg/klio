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

package podexec

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
)

// ListTier1WALFiles returns the WAL segment file names stored in tier1 for a
// cluster, sorted ascending. Partial files are excluded.
//
// WAL files live at /klio/data/wal/{clusterName}/{16-char-prefix}/{24-char-name} on
// the server pod; 'find' is unavailable in the minimal container, so we rely on
// shell globbing.
func ListTier1WALFiles(
	ctx context.Context,
	r *resources.Resources,
	namespace, podName, clusterName string,
) []string {
	var stdout, stderr bytes.Buffer
	listCmd := []string{
		"sh", "-c",
		fmt.Sprintf("ls /klio/data/wal/%s/*/0000* 2>/dev/null | sort", clusterName),
	}

	// ls exits non-zero when nothing matches, which is fine: we return an empty
	// list in that case.
	_ = r.ExecInPod(ctx, namespace, podName, serverContainerName, listCmd, &stdout, &stderr)

	output := strings.TrimSpace(stdout.String())
	if output == "" {
		return nil
	}

	var walFiles []string
	for line := range strings.SplitSeq(output, "\n") {
		parts := strings.Split(line, "/")
		name := parts[len(parts)-1]
		if strings.HasSuffix(name, ".partial") {
			continue
		}
		walFiles = append(walFiles, name)
	}

	return walFiles
}

// WALsOlderThan returns the entries of walFiles that are strictly older than
// boundary. WAL segment names are fixed-width hex, so a lexicographic
// comparison matches WAL ordering.
func WALsOlderThan(walFiles []string, boundary string) []string {
	var older []string
	for _, w := range walFiles {
		if w < boundary {
			older = append(older, w)
		}
	}

	return older
}
