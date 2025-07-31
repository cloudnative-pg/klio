package conditions

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/e2e-framework/klient/k8s"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	waitConditions "sigs.k8s.io/e2e-framework/klient/wait/conditions"
)

// KlioServerIsReady checks if the given KlioServer is ready by checking the readiness of its pod.
func KlioServerIsReady(r *resources.Resources, server k8s.Object) wait.ConditionWithContextFunc {
	// TODO: This is a temporary solution, we should use the KlioServer controller to manage the readiness of the server.
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      server.GetName() + "-klio-0",
			Namespace: server.GetNamespace(),
		},
	}

	return waitConditions.New(r).PodReady(pod)
}
