package conditions

import (
	"context"
	"fmt"

	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/e2e-framework/klient/k8s"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
)

// ClusterIsReady checks if the given CloudNativePG cluster is ready.
func ClusterIsReady(r *resources.Resources, cluster k8s.Object) wait.ConditionWithContextFunc {
	return func(ctx context.Context) (bool, error) {
		if err := r.Get(ctx, cluster.GetName(), cluster.GetNamespace(), cluster); err != nil {
			return false, fmt.Errorf("failed to get Cluster: %w", err)
		}
		cluster, ok := cluster.(*cnpgv1.Cluster)
		if !ok {
			return false, fmt.Errorf("object is not a Cluster: %v", cluster)
		}
		if cluster.Status.ReadyInstances != cluster.Spec.Instances {
			return false, nil
		}
		for _, condition := range cluster.Status.Conditions {
			if condition.Type == string(cnpgv1.ConditionClusterReady) {
				return string(condition.Status) == string(cnpgv1.ConditionTrue), nil
			}
		}

		return false, nil
	}
}

// BackupIsCompleted checks if the given backup is completed.
func BackupIsCompleted(r *resources.Resources, backup k8s.Object) wait.ConditionWithContextFunc {
	return func(ctx context.Context) (bool, error) {
		if err := r.Get(ctx, backup.GetName(), backup.GetNamespace(), backup); err != nil {
			return false, fmt.Errorf("failed to get Backup: %w", err)
		}
		backup, ok := backup.(*cnpgv1.Backup)
		if !ok {
			return false, fmt.Errorf("object is not a Backup: %v", backup)
		}
		if backup.Status.Phase == cnpgv1.BackupPhaseCompleted {
			return true, nil
		}

		return false, nil
	}
}

// InitContainerHasRestarted checks if an init container has restarted
// by comparing the current restart count against the initial count.
func InitContainerHasRestarted(
	r *resources.Resources,
	podName string,
	namespace string,
	containerName string,
	initialRestartCount int32,
) wait.ConditionWithContextFunc {
	return func(ctx context.Context) (bool, error) {
		var pod corev1.Pod
		if err := r.Get(ctx, podName, namespace, &pod); err != nil {
			return false, fmt.Errorf("failed to get pod: %w", err)
		}

		// Find the specified init container
		for _, containerStatus := range pod.Status.InitContainerStatuses {
			if containerStatus.Name == containerName {
				if containerStatus.RestartCount > initialRestartCount {
					return true, nil
				}

				return false, nil
			}
		}

		return false, fmt.Errorf("init container %s not found in pod", containerName)
	}
}
