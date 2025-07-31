package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	"github.com/cloudnative-pg/machinery/pkg/api"
	"github.com/foomo/htpasswd"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/envconf"

	kliov1alpha1 "github.com/cloudnative-pg/klio/operator/api/v1alpha1"
	machineryConditions "github.com/cloudnative-pg/klio/operator/test/machinery/pkg/conditions"
	machineryFeatures "github.com/cloudnative-pg/klio/operator/test/machinery/pkg/features"
	"github.com/cloudnative-pg/klio/operator/test/utils/conditions"
)

func newBackupFeature(
	name string, backupTarget cnpgv1.BackupTarget, instances int, namespace string,
) *machineryFeatures.BackupFeature {
	const clusterName = "test-cluster"
	const klioServerName = "test-klio-server"

	namespaceObj := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: namespace},
	}

	issuer := &certmanagerv1.Issuer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "selfsigned-issuer",
			Namespace: namespace,
		},
		Spec: certmanagerv1.IssuerSpec{
			IssuerConfig: certmanagerv1.IssuerConfig{
				SelfSigned: &certmanagerv1.SelfSignedIssuer{},
			},
		},
	}

	certificate := &certmanagerv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: namespace,
		},
		Spec: certmanagerv1.CertificateSpec{
			CommonName: "test",
			DNSNames:   []string{klioServerName + "." + namespace},
			SecretName: "test-tls",
			IssuerRef: cmmeta.ObjectReference{
				Name:  issuer.Name,
				Kind:  issuer.Kind,
				Group: issuer.GroupVersionKind().Group,
			},
			IsCA: false,
			Usages: []certmanagerv1.KeyUsage{
				certmanagerv1.UsageServerAuth,
			},
		},
	}

	clientSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "klio-client",
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"username": []byte("klio"),
			"password": []byte("testclientpassword123"),
		},
	}

	cnpgCluster := &cnpgv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterName,
			Namespace: namespace,
		},
		Spec: cnpgv1.ClusterSpec{
			Instances: instances,
			StorageConfiguration: cnpgv1.StorageConfiguration{
				Size: "1Gi",
			},
			LogLevel: "debug",
			PostgresConfiguration: cnpgv1.PostgresConfiguration{
				PgHBA: []string{"local replication all peer"},
			},
			Plugins: []cnpgv1.PluginConfiguration{{
				Name:          "klio.cnpg.io",
				Enabled:       ptr.To(true),
				IsWALArchiver: ptr.To(true),
				Parameters: map[string]string{
					"serverAddress":    certificate.Spec.DNSNames[0],
					"clientSecretName": clientSecret.GetName(),
					"serverSecretName": certificate.Spec.SecretName,
				},
			}},
		},
	}

	hp := htpasswd.HashedPasswords{}
	_ = hp.SetPassword(
		fmt.Sprintf("%v@%v", string(clientSecret.Data["username"]), clusterName),
		string(clientSecret.Data["password"]),
		htpasswd.HashBCrypt,
	)

	userSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-user",
			Namespace: namespace,
		},
		Immutable: nil,
		Data: map[string][]byte{
			"htpasswd": hp.Bytes(),
		},
	}

	encryptionSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "encryption",
			Namespace: namespace,
		},
		Immutable: nil,
		Data: map[string][]byte{
			"password": []byte("testecryptionpassword123"),
		},
	}

	klioServer := &kliov1alpha1.Server{
		ObjectMeta: metav1.ObjectMeta{
			Name:      klioServerName,
			Namespace: namespace,
		},
		Spec: kliov1alpha1.ServerSpec{
			BaseConfiguration: kliov1alpha1.BaseConfiguration{},
			Image:             "registry.dev:5000/klio-testing:dev",
			ImagePullPolicy:   corev1.PullAlways,
			TLSSecretName:     certificate.Spec.SecretName,
			CacheConfiguration: kliov1alpha1.CacheConfiguration{
				PersistentVolumeClaimTemplate: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOncePod},
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse("1Gi"),
						},
					},
				},
			},
			DataConfiguration: kliov1alpha1.DataConfiguration{
				PersistentVolumeClaimTemplate: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOncePod},
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse("1Gi"),
						},
					},
				},
			},
			Password: &cnpgv1.SecretKeySelector{
				LocalObjectReference: api.LocalObjectReference{
					Name: encryptionSecret.GetName(),
				},
				Key: "password",
			},
			Users: corev1.LocalObjectReference{
				Name: userSecret.GetName(),
			},
		},
	}

	backup := &cnpgv1.Backup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-backup",
			Namespace: namespace,
		},
		Spec: cnpgv1.BackupSpec{
			Cluster: cnpgv1.LocalObjectReference{
				Name: clusterName,
			},
			Target: backupTarget,
			Method: cnpgv1.BackupMethodPlugin,
			PluginConfiguration: &cnpgv1.BackupPluginConfiguration{
				Name: cnpgCluster.Spec.Plugins[0].Name,
			},
		},
	}

	setupFunc := func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		t.Helper()
		t.Logf("Creating resources for backup feature: %s", name)
		r, err := resources.New(cfg.Client().RESTConfig())
		require.NoError(t, err, "failed to create resources client")
		require.NoError(t, r.Create(ctx, namespaceObj), "failed to create namespace")
		require.NoError(t, r.Create(ctx, clientSecret), "failed to create client secret")
		require.NoError(t, r.Create(ctx, cnpgCluster), "failed to create CNPG cluster")
		require.NoError(t, r.Create(ctx, userSecret), "failed to create user secret")
		require.NoError(t, r.Create(ctx, encryptionSecret), "failed to create encryption secret")
		require.NoError(t, r.Create(ctx, issuer), "failed to create issuer")
		require.NoError(t, r.Create(ctx, certificate), "failed to create certificate")
		require.NoError(t, r.Create(ctx, klioServer), "failed to create KLIO server")

		t.Logf("Waiting for resources to be ready for backup feature: %s", name)
		err = wait.For(
			machineryConditions.ClusterIsReady(r, cnpgCluster),
			wait.WithTimeout(2*time.Minute),
			wait.WithInterval(10*time.Second),
		)
		require.NoError(t, err, "failed to wait for CNPG cluster to be ready")

		err = wait.For(
			conditions.KlioServerIsReady(r, klioServer),
			wait.WithTimeout(2*time.Minute),
			wait.WithInterval(10*time.Second),
		)
		require.NoError(t, err, "failed to wait for Klio server to be ready")
		t.Logf("Resources created and ready for backup feature: %s", name)

		return ctx
	}

	teardownFunc := func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		t.Helper()
		t.Logf("Tearing down resources for backup feature: %s", name)
		r, err := resources.New(cfg.Client().RESTConfig())
		require.NoError(t, err, "failed to create resources client")
		require.NoError(t, r.Delete(ctx, namespaceObj), "failed to delete namespace")
		t.Logf("Resources torn down for backup feature: %s", name)

		return ctx
	}

	return machineryFeatures.NewBackupFeature(name,
		setupFunc,
		backup,
		teardownFunc,
		machineryFeatures.WithTimeout(30*time.Second))
}

func BackupFromPrimary(namespace string) *machineryFeatures.BackupFeature {
	return newBackupFeature("BackupFromPrimary", cnpgv1.BackupTargetPrimary, 1, namespace)
}

func BackupFromStandby(namespace string) *machineryFeatures.BackupFeature {
	return newBackupFeature("BackupFromStandby", cnpgv1.BackupTargetStandby, 2, namespace)
}
