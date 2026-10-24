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

package kopiaexec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"

	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
)

const (
	// Tier1ConfigPattern matches the password files of the tier1 Kopia
	// configs the Klio server writes in its pod.
	Tier1ConfigPattern = "/tmp/kopiaconfig_tier1_*.kopia-password"
	// Tier2RWConfigPattern matches the password files of the read-write
	// tier2 Kopia configs the Klio server writes in its pod.
	Tier2RWConfigPattern = "/tmp/kopiaconfig_tier2_rw_*.kopia-password"

	passwordFileSuffix = ".kopia-password"
)

// ErrConfigNotFound is returned when no Kopia config matches a pattern.
var ErrConfigNotFound = errors.New("no kopia config file found")

// ConfigFile returns the path of a Kopia config in the given pod, found
// through its password file matching pattern. Every config the server wrote
// points to the same repository, so the first match is used.
func ConfigFile(
	ctx context.Context,
	r *resources.Resources,
	namespace, podName, containerName, pattern string,
) (string, error) {
	var stdout, stderr bytes.Buffer
	findCmd := []string{"sh", "-c", "ls " + pattern + " 2>/dev/null"}
	if err := r.ExecInPod(ctx, namespace, podName, containerName, findCmd, &stdout, &stderr); err != nil {
		return "", fmt.Errorf("failed to locate kopia config %q: %w; stderr: %s", pattern, err, stderr.String())
	}

	passwordFile, _, _ := strings.Cut(strings.TrimSpace(stdout.String()), "\n")
	if passwordFile == "" {
		return "", fmt.Errorf("%w: matching %q", ErrConfigNotFound, pattern)
	}

	return strings.TrimSuffix(passwordFile, passwordFileSuffix), nil
}
