package controller

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	kliov1alpha1 "github.com/cloudnative-pg/klio/operator/api/v1alpha1"
)

// ServerReconciler reconciles a Server object.
type ServerReconciler struct {
	client.Client

	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=klio.cnpg.io,resources=pluginconfigurations,verbs=get;list;watch
// +kubebuilder:rbac:groups=klio.cnpg.io,resources=servers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=klio.cnpg.io,resources=servers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=klio.cnpg.io,resources=servers/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update

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

	if err := r.reconcile(ctx, &server); err != nil {
		return ctrl.Result{}, err
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
		//nolint:godox
		// TODO: we should probably add a way for the user to let the secrets passed to be watched
		Complete(r)
	if err != nil {
		return fmt.Errorf("failed setting up the server controller: %w", err)
	}

	return nil
}
