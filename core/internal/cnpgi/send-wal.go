package cnpgi

import (
	"context"
	"errors"
	"fmt"
	"time"

	cnpgv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/machinery/pkg/log"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/cloudnative-pg/klio/core/internal/supervisor"
)

// SendWalController is the controller that starts the send-wal
// subprocess when the Pod represent a primary or a designated
// primary for replica clusters.
type SendWalController struct {
	Scheme *runtime.Scheme

	KlioConfigFile string
	PodName        string
	ClusterKey     types.NamespacedName
}

// ErrInstanceIsReplica is raised as a send-wal process cause when
// this instance got demoted.
var ErrInstanceIsReplica = errors.New("instance is a replica")

// Start starts the informers and handle the watch events.
func (m *SendWalController) Start(ctx context.Context) error {
	contextLogger := log.FromContext(ctx)

	// Step 1: configuration validation
	if m.PodName == "" {
		return InvalidControllerConfigurationError{
			msg: "missing pod name",
		}
	}

	if m.ClusterKey.Name == "" {
		return InvalidControllerConfigurationError{
			msg: "missing cluster name",
		}
	}

	if m.ClusterKey.Namespace == "" {
		return InvalidControllerConfigurationError{
			msg: "missing cluster namespace",
		}
	}

	// Step 2: supervisor setup
	sendWALArgs := []string{
		"send-wal",
	}
	if m.KlioConfigFile != "" {
		sendWALArgs = append(sendWALArgs, "--primary=false", "--config", m.KlioConfigFile)
	}
	sendWal := supervisor.NewService(&supervisor.Definition{
		Exec:              "klio",
		Args:              sendWALArgs,
		AutoRestart:       true,
		RestartWaitPeriod: 15 * time.Second,
	})

	// Step 3: controller manager setup
	controllerOptions := ctrl.Options{
		Scheme: m.Scheme,
		Cache: cache.Options{
			ByObject: map[client.Object]cache.ByObject{
				&cnpgv1.Cluster{}: {
					Field: fields.OneTermEqualSelector("metadata.name", m.ClusterKey.Name),
					Namespaces: map[string]cache.Config{
						m.ClusterKey.Namespace: {},
					},
				},
			},
		},
		Client: client.Options{
			Cache: &client.CacheOptions{
				DisableFor: []client.Object{
					&cnpgv1.Cluster{},
				},
			},
		},
		Metrics: server.Options{
			// Disable the metrics endpoint since we're using OTEL bridge
			BindAddress: "0",
		},
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), controllerOptions)
	if err != nil {
		contextLogger.Error(err, "unable to start manager")
		return fmt.Errorf("while creating controller manager: %w", err)
	}

	sendWalReconciler := SendWalClusterReconciler{
		PodName:    m.PodName,
		Client:     mgr.GetClient(),
		Supervisor: sendWal,
	}
	if err := sendWalReconciler.SetupWithManager(mgr); err != nil {
		contextLogger.Error(err, "unable to setup send-wal reconciler")
		return fmt.Errorf("while setting up send-wal reconciler: %w", err)
	}

	if err := mgr.Add(sendWal); err != nil {
		contextLogger.Error(err, "unable to setup send-wal subprocess manager")
		return fmt.Errorf("while adding send-wal subprocess manager: %w", err)
	}

	contextLogger.Info(
		"Starting manager for klio send-wal, waiting API server events before starting subprocess",
		"cluster", m.ClusterKey,
		"podName", m.PodName,
		"klioConfigFile", m.KlioConfigFile,
	)

	if err := mgr.Start(ctx); err != nil {
		return fmt.Errorf("controller manager error: %w", err)
	}

	return nil
}

// SendWalClusterReconciler reconciles a Server object.
type SendWalClusterReconciler struct {
	client.Client

	// PodName is the name of this instance
	PodName string

	// Supervisor is the supervisor that is managing the send-wal process
	Supervisor *supervisor.Service
}

// SetupWithManager sets up the controller with the Manager.
func (r *SendWalClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	err := ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 10,
		}).
		For(&cnpgv1.Cluster{}).
		Named("cluster").
		Complete(r)
	if err != nil {
		return fmt.Errorf("failed setting up the server controller: %w", err)
	}

	return nil
}

// Reconcile is invoked every time something changes in the cluster.
func (r *SendWalClusterReconciler) Reconcile(
	ctx context.Context,
	req reconcile.Request,
) (reconcile.Result, error) {
	sendWALRunning := r.Supervisor.IsProcessRunning()

	contextLogger := log.FromContext(ctx).WithValues("name", req.Name, "namespace", req.Namespace)
	contextLogger.Trace(
		"Received request",
		"sendWALRunning", sendWALRunning,
	)

	// if the context has already been cancelled,
	// trying to reconcile would just lead to misleading errors being reported
	if err := ctx.Err(); err != nil {
		contextLogger.Warning("Context cancelled, will not reconcile", "err", err)
		return ctrl.Result{}, nil
	}

	cluster := cnpgv1.Cluster{}
	if err := r.Get(ctx, req.NamespacedName, &cluster); err != nil {
		if apierrors.IsNotFound(err) {
			if sendWALRunning {
				contextLogger.Info("cluster not found and send-wal is running, stopping send-wal")
				_ = r.Supervisor.EnsureProcessStopped(ctx, errors.New("cluster not found"))
			}

			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, err
	}

	// no configuration changes when switching over
	if cluster.Status.CurrentPrimary != cluster.Status.TargetPrimary {
		contextLogger.Info(
			"Switchover or failover is in progress, waiting for it to finish",
			"currentPrimary", cluster.Status.CurrentPrimary,
			"targetPrimary", cluster.Status.TargetPrimary,
		)

		return reconcile.Result{}, nil
	}

	// if I'm a primary, my send-wal process need to be up
	isPrimary := r.PodName == cluster.Status.CurrentPrimary
	switch {
	case isPrimary && !sendWALRunning:
		return ctrl.Result{}, r.Supervisor.EnsureProcessStarted(ctx)

	case !isPrimary && sendWALRunning:
		return ctrl.Result{}, r.Supervisor.EnsureProcessStopped(ctx, ErrInstanceIsReplica)
	}

	return reconcile.Result{}, nil
}

// InvalidControllerConfigurationError is raised when the controller configuration
// was not valid.
type InvalidControllerConfigurationError struct {
	msg string
}

// Error implements the error interface.
func (e InvalidControllerConfigurationError) Error() string {
	return fmt.Sprintf("controller configuration is not valid: %s ", e.msg)
}
