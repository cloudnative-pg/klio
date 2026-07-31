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

package controller

import (
	"context"
	"fmt"

	"github.com/cloudnative-pg/machinery/pkg/log"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/yaml"

	kliov1alpha1 "github.com/cloudnative-pg/klio/operator/api/v1alpha1"
	"github.com/cloudnative-pg/klio/operator/internal/klioconfig"
)

// PluginConfigurationReconciler reconciles a PluginConfiguration object.
type PluginConfigurationReconciler struct {
	client.Client

	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=klio.cnpg.io,resources=pluginconfigurations,verbs=get;list;watch
// +kubebuilder:rbac:groups=klio.cnpg.io,resources=pluginconfigurations/status,verbs=get;patch;update
// +kubebuilder:rbac:groups=klio.cnpg.io,resources=pluginconfigurations/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update

// Reconcile handles PluginConfiguration changes by creating or updating the
// per-PC klio-config secret.
func (r *PluginConfigurationReconciler) Reconcile(
	ctx context.Context,
	req reconcile.Request,
) (reconcile.Result, error) {
	contextLogger := log.FromContext(ctx).WithValues(
		"pluginConfiguration", req.NamespacedName,
	)

	// Fetch the PluginConfiguration
	var pc kliov1alpha1.PluginConfiguration
	if err := r.Get(ctx, req.NamespacedName, &pc); err != nil {
		if apierrors.IsNotFound(err) {
			contextLogger.Debug("PluginConfiguration not found, ignoring")
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, fmt.Errorf("while getting PluginConfiguration: %w", err)
	}

	// Generate config using PC name as configKey; clusterName is always set in the spec
	klioConfig := klioconfig.GenerateConfig(pc.Spec, pc.Name)

	yamlData, err := yaml.Marshal(klioConfig)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("while marshalling config: %w", err)
	}

	// Create or update the secret named after the PC
	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pc.Name,
			Namespace: pc.Namespace,
		},
	}

	operationResult, err := controllerutil.CreateOrUpdate(ctx, r.Client, &secret, func() error {
		if err := controllerutil.SetControllerReference(&pc, &secret, r.Scheme); err != nil {
			return err
		}

		if secret.Labels == nil {
			secret.Labels = make(map[string]string)
		}
		secret.Labels[klioconfig.TypeLabelKey] = klioconfig.TypeLabelValue

		secret.Data = map[string][]byte{
			klioconfig.ConfigDataKey: yamlData,
		}

		return nil
	})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("while creating/updating secret %q: %w", pc.Name, err)
	}

	// Set status condition only when the Secret was actually created or updated
	if operationResult != controllerutil.OperationResultNone {
		message := fmt.Sprintf("Configuration Secret %s/%s %s", pc.Namespace, pc.Name, operationResult)
		apimeta.SetStatusCondition(&pc.Status.Conditions, metav1.Condition{
			Type:               kliov1alpha1.PluginConfigurationConditionConfigurationApplied,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: pc.Generation,
			Reason:             kliov1alpha1.ReasonSecretUpdated,
			Message:            message,
		})

		if err := r.Status().Update(ctx, &pc); err != nil {
			contextLogger.Error(err, "Failed to update status after successful reconciliation")
			return ctrl.Result{}, fmt.Errorf("while updating status: %w", err)
		}
	}

	contextLogger.Info("Reconciled klio-config secret", "secret", pc.Name, "operation", operationResult)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PluginConfigurationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kliov1alpha1.PluginConfiguration{}).
		Named("pluginconfiguration").
		Complete(r)
}
