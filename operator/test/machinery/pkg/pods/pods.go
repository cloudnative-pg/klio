// Package pods provides utilities for working with Kubernetes pods in e2e tests.
package pods

import (
	"context"
	"io"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
)

// GetLogs retrieves logs from a specific container in a pod.
func GetLogs(
	ctx context.Context,
	res *resources.Resources,
	pod *corev1.Pod,
	containerName string,
) (string, error) {
	clientset, err := kubernetes.NewForConfig(res.GetConfig())
	if err != nil {
		return "", err
	}

	req := clientset.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{
		Container: containerName,
	})

	stream, err := req.Stream(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = stream.Close() }()

	logs, err := io.ReadAll(stream)
	if err != nil {
		return "", err
	}

	return string(logs), nil
}
