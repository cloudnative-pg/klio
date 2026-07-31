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

package postgres

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
)

// ExecPostgresQuery executes a query in a target PostgreSQL pod.
func ExecPostgresQuery(
	ctx context.Context,
	res *resources.Resources,
	pod *corev1.Pod,
	databaseName string,
	query string,
) (string, error) {
	var stdout, stderr bytes.Buffer
	cmd := []string{
		"psql",
		"-U", "postgres",
		"-tA",
		"-c", query,
		databaseName,
	}

	err := res.ExecInPod(ctx, pod.Namespace, pod.Name, "postgres", cmd, &stdout, &stderr)
	if err != nil {
		return "", fmt.Errorf("while running PostgreSQL query: %w; stderr: %s", err, stderr.String())
	}

	return strings.TrimSpace(stdout.String()), nil
}

// CheckpointAndSwitchWal executes a checkpoint and switches WAL file in a target PostgreSQL pod.
func CheckpointAndSwitchWal(ctx context.Context, res *resources.Resources, pod *corev1.Pod) error {
	_, err := ExecPostgresQuery(ctx, res, pod, "postgres",
		"CHECKPOINT; SELECT pg_switch_wal();")

	return err
}
