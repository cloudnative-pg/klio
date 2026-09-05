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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	kliov1alpha1 "github.com/cloudnative-pg/klio/operator/api/v1alpha1"
)

// ServerReconciler reconciles a Server object.
type ServerReconciler struct {
	client.Client

	Scheme                         *runtime.Scheme
	Recorder                       events.EventRecorder
	HaveSecurityContextConstraints bool
}

// +kubebuilder:rbac:groups=klio.cnpg.io,resources=pluginconfigurations,verbs=get;list;watch
// +kubebuilder:rbac:groups=klio.cnpg.io,resources=servers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=klio.cnpg.io,resources=servers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=klio.cnpg.io,resources=servers/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update
// +kubebuilder:rbac:groups="",resources=events,verbs=create
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch;patch;delete

//nolint:godox
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Server object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ServerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	contextLogger := logf.FromContext(ctx).WithValues("namespace", req.Namespace, "name", req.Name)
	contextLogger.V(1).Info("Reconciling Klio Server")

	var server kliov1alpha1.Server
	if err := r.Get(ctx, req.NamespacedName, &server); err != nil {
		if errors.IsNotFound(err) {
			contextLogger.V(1).Info("Klio Server not found, nothing to do")
			return ctrl.Result{}, nil
		}
		contextLogger.Error(err, "Failed to get Klio Server")

		return ctrl.Result{}, fmt.Errorf("failed to get Klio Server: %w", err)
	}

	if server.DeletionTimestamp != nil {
		contextLogger.V(4).Info("Klio Server is being deleted, nothing to do")
		return ctrl.Result{}, nil
	}

	if result, err := r.reconcile(ctx, &server); err != nil || !result.IsZero() {
		return result, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ServerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	err := ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 10,
		}).
		For(&kliov1alpha1.Server{}).
		Named("server").
		Owns(&appsv1.StatefulSet{}).
		Watches(
			&corev1.PersistentVolumeClaim{},
			handler.EnqueueRequestsFromMapFunc(r.findServerForPVC),
			builder.WithPredicates(predicate.NewPredicateFuncs(func(obj client.Object) bool {
				_, exists := obj.GetLabels()[klioServerLabel]
				return exists
			})),
		).
		//nolint:godox
		// TODO: we should probably add a way for the user to let the secrets passed to be watched
		Complete(r)
	if err != nil {
		return fmt.Errorf("failed setting up the server controller: %w", err)
	}

	return nil
}
