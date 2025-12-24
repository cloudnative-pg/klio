package controller

import (
	"context"
	"fmt"

	"github.com/cloudnative-pg/machinery/pkg/log"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	kliov1alpha1 "github.com/cloudnative-pg/klio/operator/api/v1alpha1"
)

// RecoverySourceReconciler reconciles a RecoverySource object.
type RecoverySourceReconciler struct {
	client.Client

	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=klio.cnpg.io,resources=recoverysources,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=klio.cnpg.io,resources=recoverysources/status,verbs=get;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer
// to the desired state.
func (r *RecoverySourceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	contextLogger := log.FromContext(ctx).WithValues("namespace", req.Namespace, "name", req.Name)
	contextLogger.Info("Reconciling Klio Recovery Source")

	var recoverySource kliov1alpha1.RecoverySource
	if err := r.Get(ctx, req.NamespacedName, &recoverySource); err != nil {
		if errors.IsNotFound(err) {
			contextLogger.Info("Klio Recovery Source not found, nothing to do")
			return ctrl.Result{}, nil
		}
		contextLogger.Error(err, "Failed to get Klio Recovery Source")

		return ctrl.Result{}, fmt.Errorf("failed to get Klio Recovery Source: %w", err)
	}

	if recoverySource.DeletionTimestamp != nil {
		contextLogger.Info("Klio Recovery Source is being deleted, nothing to do")
		return ctrl.Result{}, nil
	}

	if err := r.reconcile(ctx, &recoverySource); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RecoverySourceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	err := ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 10,
		}).
		For(&kliov1alpha1.RecoverySource{}).
		Named("recoverysource").
		Owns(&appsv1.StatefulSet{}).
		//nolint:godox
		// TODO: we should probably add a way for the user to let the secrets passed to be watched
		Complete(r)
	if err != nil {
		return fmt.Errorf("failed setting up the recovery source controller: %w", err)
	}

	return nil
}
