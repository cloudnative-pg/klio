package conditions

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/e2e-framework/klient/k8s"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"

	kliov1alpha1 "github.com/cloudnative-pg/klio/operator/api/v1alpha1"
)

// PluginConfigurationHasCondition checks if the PluginConfiguration has the specified condition
// with the expected status and observed generation.
func PluginConfigurationHasCondition(
	r *resources.Resources,
	pc k8s.Object,
	conditionType string,
	expectedStatus metav1.ConditionStatus,
	expectedGeneration int64,
) wait.ConditionWithContextFunc {
	return func(ctx context.Context) (bool, error) {
		if err := r.Get(ctx, pc.GetName(), pc.GetNamespace(), pc); err != nil {
			return false, fmt.Errorf("failed to get PluginConfiguration: %w", err)
		}

		pluginConfig, ok := pc.(*kliov1alpha1.PluginConfiguration)
		if !ok {
			return false, fmt.Errorf("object is not a PluginConfiguration: %v", pc)
		}

		for _, condition := range pluginConfig.Status.Conditions {
			if condition.Type == conditionType {
				statusMatch := condition.Status == expectedStatus
				generationMatch := condition.ObservedGeneration == expectedGeneration
				return statusMatch && generationMatch, nil
			}
		}

		return false, nil
	}
}
