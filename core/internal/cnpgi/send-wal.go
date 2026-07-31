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

package cnpgi

import (
	"context"
	"errors"
	"fmt"
	"time"

	cnpgv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/machinery/pkg/log"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/cloudnative-pg/klio/core/internal/supervisor"
)

// ErrInstanceIsReplica is raised as a send-wal process cause when
// this instance got demoted.
var ErrInstanceIsReplica = errors.New("instance is a replica")

// SendWalClusterReconciler reconciles a Cluster object.
type SendWalClusterReconciler struct {
	client.Client

	// PodName is the name of this instance
	PodName string

	KlioConfigFile string

	// supervisor is the supervisor that is managing the send-wal process
	supervisor *supervisor.Service
}

// SetupWithManager sets up the controller with the Manager.
func (r *SendWalClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	sendWALArgs := []string{
		"send-wal",
	}
	if r.KlioConfigFile != "" {
		sendWALArgs = append(sendWALArgs, "--primary=false", "--config", r.KlioConfigFile)
	}

	sendWal := supervisor.NewService(&supervisor.Definition{
		Exec:              "klio",
		Args:              sendWALArgs,
		AutoRestart:       true,
		RestartWaitPeriod: 15 * time.Second,
	})
	r.supervisor = sendWal

	if err := mgr.Add(sendWal); err != nil {
		return fmt.Errorf("failed adding send-wal sub-process supervisor to controller: %w", err)
	}

	err := ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 10,
		}).
		For(&cnpgv1.Cluster{}).
		Named("cluster").
		Complete(r)
	if err != nil {
		return fmt.Errorf("failed setting up the cluster controller: %w", err)
	}

	return nil
}

// Reconcile is invoked every time something changes in the cluster.
func (r *SendWalClusterReconciler) Reconcile(
	ctx context.Context,
	req reconcile.Request,
) (reconcile.Result, error) {
	sendWALRunning := r.supervisor.IsProcessRunning()

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
				_ = r.supervisor.EnsureProcessStopped(ctx, errors.New("cluster not found"))
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
		return ctrl.Result{}, r.supervisor.EnsureProcessStarted(ctx)

	case !isPrimary && sendWALRunning:
		return ctrl.Result{}, r.supervisor.EnsureProcessStopped(ctx, ErrInstanceIsReplica)
	}

	return reconcile.Result{}, nil
}
