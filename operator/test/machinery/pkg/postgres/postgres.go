package postgres

import (
	"bytes"
	"fmt"
	"strings"

	"golang.org/x/net/context"
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
