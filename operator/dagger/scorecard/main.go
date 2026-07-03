// A module for running operator-sdk scorecard against an OLM bundle.
//
// operator-sdk scorecard (https://sdk.operatorframework.io/docs/testing-operators/scorecard/)
// validates an operator bundle by running its basic and OLM test suites as pods
// on a Kubernetes cluster. This module spins up an ephemeral k3s cluster inside
// Dagger and runs scorecard against it, so no pre-existing cluster is required.

package main

import (
	"context"
	"fmt"
	"time"

	"dagger/scorecard/internal/dagger"
)

// waitForDefaultServiceAccount polls until the default namespace's "default"
// service account exists. k3s opens its API port (so Server().Start returns)
// before the API server is actually serving — early requests get a 503
// "runtime core not ready" — and before the ServiceAccount controller creates
// the default service account, which scorecard's test pods need to pass
// admission. `kubectl wait` fails immediately on the 503 instead of retrying,
// so poll instead. The k3s Kubectl helper prepends "kubectl " to these args,
// so the first "get" is the initial attempt and the loop retries.
const waitForDefaultServiceAccount = "get serviceaccount default -n default >/dev/null 2>&1 && exit 0; " +
	"for _ in $(seq 1 90); do sleep 2; " +
	"kubectl get serviceaccount default -n default >/dev/null 2>&1 && exit 0; " +
	"done; " +
	"echo 'timed out waiting for the default service account' >&2; exit 1"

// readyTimeout bounds how long we wait for the k3s cluster to become ready.
// If the k3s server crashes on startup (e.g. the host doesn't delegate the
// cpu cgroup v2 controller), the k3s module's kubeconfig wait loop has no
// timeout of its own and spins forever, silently burning the whole CI step
// budget instead of failing fast. This is well above the ~15s a healthy
// cluster takes and the 180s worst case waitForDefaultServiceAccount already
// tolerates, but well below the CI step's own timeout.
const readyTimeout = 4 * time.Minute

// Scorecard runs operator-sdk scorecard on an ephemeral k3s cluster.
type Scorecard struct {
	// OperatorSDKImage is the operator-sdk image that provides the scorecard
	// command. It must match the version used to generate the bundle.
	// +private
	OperatorSDKImage string
	// K3SImage is the k3s image used for the ephemeral cluster.
	// +private
	K3SImage string
}

// New creates a Scorecard module.
func New(
	// operatorSdkImage is the operator-sdk image providing the scorecard command.
	// renovate image: datasource=docker depName=quay.io/operator-framework/operator-sdk versioning=docker
	// +default="quay.io/operator-framework/operator-sdk:v1.42.3@sha256:b7378eb179b97fbdf1ff1304e86f62e9ff9f9c2d0089464111ce8eddd39e2013"
	// +optional
	operatorSdkImage string,
	// k3SImage is the k3s image used for the ephemeral cluster.
	// renovate image: datasource=docker depName=k3s lookupName=rancher/k3s versioning=docker
	// +default="rancher/k3s:v1.36.2-k3s1@sha256:6a47cea22c4b834d4ba72c89d291696b79ebe406251f90b446e4dff03513dd87"
	// +optional
	k3SImage string,
) *Scorecard {
	return &Scorecard{
		OperatorSDKImage: operatorSdkImage,
		K3SImage:         k3SImage,
	}
}

// Run executes the scorecard basic and OLM test suites against the given OLM
// bundle on an ephemeral k3s cluster and returns the text report on stdout.
// scorecard exits non-zero when any test does not pass, which fails this
// function directly; no separate verdict evaluation is needed.
func (m *Scorecard) Run(
	ctx context.Context,
	// bundle is the generated OLM bundle directory to validate.
	bundle *dagger.Directory,
	// waitTime is how long scorecard waits for each test pod to complete.
	// +default="300s"
	// +optional
	waitTime string,
) (string, error) {
	// Start the k3s server before reading its kubeconfig: Config polls the
	// server's shared cache for the generated kubeconfig, so the server must be
	// running first.
	k3s := dag.K3S("scorecard", dagger.K3SOpts{Image: m.K3SImage})
	if _, err := k3s.Server().Start(ctx); err != nil {
		return "", err
	}

	// Wait for the cluster to be ready enough for scorecard's test pods (see
	// waitForDefaultServiceAccount), bounded by readyTimeout so a crashed
	// server fails fast instead of hanging.
	readyCtx, cancel := context.WithTimeout(ctx, readyTimeout)
	defer cancel()
	if _, err := k3s.Kubectl(waitForDefaultServiceAccount).Sync(readyCtx); err != nil {
		return "", fmt.Errorf("k3s cluster did not become ready: %w", err)
	}

	return dag.Container().
		From(m.OperatorSDKImage).
		WithMountedFile("/kube/config", k3s.Config()).
		WithMountedDirectory("/bundle", bundle).
		WithEnvVariable("KUBECONFIG", "/kube/config").
		// Bust the cache so scorecard runs against a fresh cluster every time.
		WithEnvVariable("CACHEBUST", time.Now().String()).
		WithExec(
			[]string{"scorecard", "/bundle", "--wait-time", waitTime, "--verbose"},
			dagger.ContainerWithExecOpts{UseEntrypoint: true},
		).
		Stdout(ctx)
}
