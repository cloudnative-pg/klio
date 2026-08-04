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

package e2e

import (
	"context"
	"testing"
	"time"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/envconf"

	kliov1alpha1 "github.com/cloudnative-pg/klio/operator/api/v1alpha1"
	klioFeatures "github.com/cloudnative-pg/klio/operator/test/klio/features"
	"github.com/cloudnative-pg/klio/operator/test/klio/testconfig"
	"github.com/cloudnative-pg/klio/operator/test/machinery/pkg/namespaces"
	"github.com/cloudnative-pg/klio/operator/test/utils/conditions"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/certificates"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/klio"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/secrets"
)

// pvcResizeScenario contains all resources needed for PVC resize testing.
type pvcResizeScenario struct {
	namespace        *corev1.Namespace
	issuer           *certmanagerv1.Issuer
	certificate      *certmanagerv1.Certificate
	caCertificate    *certmanagerv1.Certificate
	caIssuer         *certmanagerv1.Issuer
	encryptionSecret *corev1.Secret
	identitySecret   *corev1.Secret
	klioServer       *kliov1alpha1.Server
	name             string
}

// Setup creates all resources for the PVC resize test.
func (s *pvcResizeScenario) Setup(
	ctx context.Context,
	t *testing.T,
	cfg *envconf.Config,
) context.Context {
	t.Helper()

	t.Logf("Creating resources for PVC resize feature: %s", s.name)
	r, err := resources.New(cfg.Client().RESTConfig())
	require.NoError(t, err, "failed to create resources client")

	// Create namespace
	createNamespace(ctx, t, r, s.namespace)

	// Create certificates and secrets
	require.NoError(t, r.Create(ctx, s.issuer), "failed to create issuer")

	err = wait.For(
		conditions.IssuerIsReady(r, s.issuer),
		wait.WithTimeout(2*time.Minute),
		wait.WithInterval(5*time.Second),
	)
	require.NoError(t, err, "issuer not ready")

	require.NoError(t, r.Create(ctx, s.caCertificate), "failed to create CA certificate")

	err = wait.For(
		conditions.CertificateIsReady(r, s.caCertificate),
		wait.WithTimeout(2*time.Minute),
		wait.WithInterval(5*time.Second),
	)
	require.NoError(t, err, "CA certificate not ready")

	require.NoError(t, r.Create(ctx, s.caIssuer), "failed to create CA issuer")

	err = wait.For(
		conditions.IssuerIsReady(r, s.caIssuer),
		wait.WithTimeout(2*time.Minute),
		wait.WithInterval(5*time.Second),
	)
	require.NoError(t, err, "CA issuer not ready")

	require.NoError(t, r.Create(ctx, s.certificate), "failed to create certificate")

	err = wait.For(
		conditions.CertificateIsReady(r, s.certificate),
		wait.WithTimeout(2*time.Minute),
		wait.WithInterval(5*time.Second),
	)
	require.NoError(t, err, "certificate not ready")

	require.NoError(t, r.Create(ctx, s.encryptionSecret), "failed to create encryption secret")

	require.NoError(t, r.Create(ctx, s.identitySecret), "failed to create identity secret")

	// Create Klio server
	require.NoError(t, r.Create(ctx, s.klioServer), "failed to create Klio server")

	// Wait for Klio server to be ready
	t.Logf("Waiting for Klio server to be ready...")
	err = wait.For(
		conditions.KlioServerIsReady(r, s.klioServer),
		wait.WithTimeout(4*time.Minute),
		wait.WithInterval(10*time.Second),
	)
	require.NoError(t, err, "Klio server not ready")

	t.Logf("All resources created and ready for PVC resize feature: %s", s.name)

	return ctx
}

// Teardown deletes all resources.
func (s *pvcResizeScenario) Teardown(
	ctx context.Context,
	t *testing.T,
	cfg *envconf.Config,
) context.Context {
	t.Helper()

	t.Logf("Tearing down resources for PVC resize feature: %s", s.name)
	r, err := resources.New(cfg.Client().RESTConfig())
	require.NoError(t, err, "failed to create resources client")
	namespaces.DumpNamespaceOnFailure(ctx, t, r, testCfg.LogDir, s.namespace.Name, testconfig.DumpedKinds())
	require.NoError(t, r.Delete(ctx, s.namespace), "failed to delete namespace")
	t.Logf("Resources torn down for PVC resize feature: %s", s.name)

	return ctx
}

// NewPVCResizeFeatureConfig creates a new PVC resize feature configuration.
func NewPVCResizeFeatureConfig(name string, namespace string) klioFeatures.PVCResizeFeatureConfig {
	const (
		klioServerName        = "klio-resize-test"
		selfSignedIssuerName  = "selfsigned-issuer"
		caCertificateName     = klioServerName + "-ca"
		caIssuerName          = caCertificateName + "-issuer"
		serverCertificateName = klioServerName + "-server"
		encryptionSecretName  = "encryption"
		encryptionPassword    = "testencryptionpassword123"
	)

	// Namespace
	namespaceObj := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: namespace},
	}

	// Issuer for all certificates
	issuer := certificates.GetSelfSignedIssuerObject(selfSignedIssuerName, namespace)

	// Certificates and secrets
	caCertificate := certificates.GetCACertificateObject(caCertificateName, namespace, issuer)
	caIssuer := certificates.GetCAIssuerObject(caIssuerName, namespace, caCertificate.Spec.SecretName)
	serverCertificate := certificates.GetCertificateObject(
		serverCertificateName, namespace, []string{klioServerName}, issuer)

	// Encryption secret
	ageSecrets := secrets.GetKlioAgeEncryptionSecrets("encryption", namespace, "testencryptionpassword123")

	// Klio Server with tier1 and queue
	klioServer := klio.GetServerObject(
		klioServerName,
		namespace,
		klio.ServerTemplateOptions{
			Image:              testCfg.ServerImage,
			StorageClass:       testCfg.StorageClass,
			TLSSecretName:      serverCertificate.Spec.SecretName,
			ClientCASecretName: caCertificate.Spec.SecretName,
			Encryption: klio.EncryptionOptions{
				EncryptionKeySecretName: ageSecrets.EncryptionKeySecret.Name,
				EncryptionKeyFileName:   "encryption-key.age",
				IdentitySecretName:      ageSecrets.IdentitySecret.Name,
				IdentityFileName:        "identity.txt",
			},
		},
	)

	scenario := &pvcResizeScenario{
		namespace:        namespaceObj,
		issuer:           issuer,
		certificate:      serverCertificate,
		caCertificate:    caCertificate,
		caIssuer:         caIssuer,
		encryptionSecret: ageSecrets.EncryptionKeySecret,
		identitySecret:   ageSecrets.IdentitySecret,
		klioServer:       klioServer,
		name:             name,
	}

	return klioFeatures.PVCResizeFeatureConfig{
		Name:         name,
		Setup:        scenario.Setup,
		Teardown:     scenario.Teardown,
		KlioServer:   klioServer,
		Namespace:    namespace,
		NewDataSize:  resource.MustParse("2Gi"),
		NewCacheSize: resource.MustParse("2Gi"),
		NewQueueSize: resource.MustParse("200Mi"),
	}
}

// PVCResize returns a PVCResizeFeature for testing PVC resize functionality.
func PVCResize(namespace string) *klioFeatures.PVCResizeFeature {
	return klioFeatures.NewPVCResizeFeature(
		NewPVCResizeFeatureConfig("PVCResize", namespace))
}
