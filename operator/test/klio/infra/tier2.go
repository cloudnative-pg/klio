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

package infra

import (
	"context"
	"testing"
	"time"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"

	kliov1alpha1 "github.com/cloudnative-pg/klio/operator/api/v1alpha1"
	"github.com/cloudnative-pg/klio/operator/test/utils/conditions"
)

// Tier2 represents the required infrastructure for Klio Tier 2.
type Tier2 struct {
	Issuer *certmanagerv1.Issuer

	// RustFS infrastructure
	RustfsSecret          *corev1.Secret
	RustfsConfigMap       *corev1.ConfigMap
	RustfsCertificate     *certmanagerv1.Certificate
	RustfsService         *corev1.Service
	RustfsDeployment      *appsv1.Deployment
	RustfsCreateBucketJob *batchv1.Job

	// Klio Server with tier2
	ServerCertificate *certmanagerv1.Certificate
	CaCertificate     *certmanagerv1.Certificate
	CaIssuer          *certmanagerv1.Issuer
	UserCertificate   *certmanagerv1.Certificate
	EncryptionSecret  *corev1.Secret
	IdentitySecret    *corev1.Secret
	KlioServer        *kliov1alpha1.Server
}

// ParallelSetup sets up RustFS and Klio Server certificates in parallel,
// then deploys the Klio Server after RustFS is ready to avoid S3 connection retries.
func (s Tier2) ParallelSetup(ctx context.Context, t *testing.T, r *resources.Resources) {
	t.Helper()

	// Prepare RustFS and Klio certificates/secrets in parallel
	g, gCtx := errgroup.WithContext(ctx)

	// Deploy RustFS infrastructure
	g.Go(func() error {
		t.Logf("Deploying RustFS infrastructure...")
		require.NoError(t, r.Create(gCtx, s.RustfsSecret), "failed to create RustFS secret")
		require.NoError(t, r.Create(gCtx, s.RustfsConfigMap), "failed to create RustFS configmap")
		require.NoError(t, r.Create(gCtx, s.Issuer), "failed to create issuer")

		// Wait for issuer to be ready before creating certificates
		err := wait.For(
			conditions.IssuerIsReady(r, s.Issuer),
			wait.WithTimeout(1*time.Minute),
			wait.WithInterval(5*time.Second),
		)
		require.NoError(t, err, "issuer not ready")

		require.NoError(t, r.Create(gCtx, s.RustfsCertificate), "failed to create RustFS certificate")

		// Wait for RustFS certificate to be ready
		err = wait.For(
			conditions.CertificateIsReady(r, s.RustfsCertificate),
			wait.WithTimeout(5*time.Minute),
			wait.WithInterval(10*time.Second),
		)
		require.NoError(t, err, "RustFS certificate not ready")

		require.NoError(t, r.Create(gCtx, s.RustfsService), "failed to create RustFS service")
		require.NoError(t, r.Create(gCtx, s.RustfsDeployment), "failed to create RustFS deployment")

		// Wait for RustFS deployment to be ready
		t.Logf("Waiting for RustFS deployment to be ready...")
		err = wait.For(
			conditions.DeploymentIsReady(r, s.RustfsDeployment),
			wait.WithTimeout(2*time.Minute),
			wait.WithInterval(10*time.Second),
		)
		require.NoError(t, err, "RustFS deployment not ready")

		// Create bucket
		t.Logf("Creating S3 bucket in RustFS...")
		require.NoError(t, r.Create(gCtx, s.RustfsCreateBucketJob), "failed to create bucket creation job")

		// Wait for bucket creation to complete
		return wait.For(
			conditions.JobIsComplete(r, s.RustfsCreateBucketJob),
			wait.WithTimeout(2*time.Minute),
			wait.WithInterval(10*time.Second),
		)
	})

	// Prepare Klio Server certificates and secrets
	g.Go(func() error {
		t.Logf("Preparing Klio Server certificates...")
		require.NoError(t, r.Create(gCtx, s.CaCertificate), "failed to create CA certificate")
		require.NoError(t, r.Create(gCtx, s.CaIssuer), "failed to create CA issuer")

		// Wait for CA certificate to be ready
		err := wait.For(
			conditions.CertificateIsReady(r, s.CaCertificate),
			wait.WithTimeout(1*time.Minute),
			wait.WithInterval(5*time.Second),
		)
		require.NoError(t, err, "CA certificate not ready")

		require.NoError(t, r.Create(gCtx, s.ServerCertificate), "failed to create server certificate")
		require.NoError(t, r.Create(gCtx, s.UserCertificate), "failed to create user certificate")

		// Wait for certificates to be ready
		err = wait.For(
			conditions.CertificateIsReady(r, s.ServerCertificate),
			wait.WithTimeout(1*time.Minute),
			wait.WithInterval(5*time.Second),
		)
		require.NoError(t, err, "server certificate not ready")

		require.NoError(t, r.Create(gCtx, s.EncryptionSecret), "failed to create encryption secret")
		require.NoError(t, r.Create(gCtx, s.IdentitySecret), "failed to create identity secret")

		return nil
	})

	// Wait for both RustFS and certificates to be ready
	require.NoError(t, g.Wait(), "Parallel setup of RustFS and Klio certificates failed")

	// Deploy Klio Server after RustFS is ready to avoid S3 connection retries
	t.Logf("Deploying Klio Server with tier2...")
	require.NoError(t, r.Create(ctx, s.KlioServer), "failed to create Klio server")

	t.Logf("Waiting for Klio Server to be ready...")
	err := wait.For(
		conditions.KlioServerIsReady(r, s.KlioServer),
		wait.WithTimeout(4*time.Minute),
		wait.WithInterval(10*time.Second),
	)
	require.NoError(t, err, "Klio server not ready")
}
