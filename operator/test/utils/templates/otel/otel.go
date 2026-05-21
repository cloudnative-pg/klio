package otel

import (
	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	// CollectorName is the name of the OTEL collector.
	CollectorName = "otel-collector"
	// CollectorImage is the OTEL collector image.
	//nolint:godot,lll
	// renovate image: datasource=docker depName=otel/opentelemetry-collector-contrib versioning=docker
	CollectorImage = "otel/opentelemetry-collector-contrib:0.152.1@sha256:fa1f6ea8dabd3042fabf4411eed7fa52f253edd8940140bec89a573e62a24eb7"
	// CollectorPort is the gRPC port for the OTEL collector.
	CollectorPort = 4317
	// CollectorMetricsPort is the Prometheus metrics port for the OTEL collector.
	CollectorMetricsPort = 9464
)

// GetCollectorServiceAccount returns a ServiceAccount for the OTEL collector.
func GetCollectorServiceAccount(namespace string) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      CollectorName,
			Namespace: namespace,
		},
	}
}

// GetCollectorClusterRole returns a ClusterRole for the OTEL collector.
func GetCollectorClusterRole(namespace string) *rbacv1.ClusterRole {
	return &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: CollectorName + "-" + namespace,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"pods", "namespaces"},
				Verbs:     []string{"get", "watch", "list"},
			},
		},
	}
}

// GetCollectorClusterRoleBinding returns a ClusterRoleBinding for the OTEL collector.
func GetCollectorClusterRoleBinding(namespace string) *rbacv1.ClusterRoleBinding {
	return &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: CollectorName + "-" + namespace,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      CollectorName,
				Namespace: namespace,
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     CollectorName + "-" + namespace,
		},
	}
}

// GetCollectorCertificate returns a Certificate for the OTEL collector.
func GetCollectorCertificate(namespace string, issuer *certmanagerv1.Issuer) *certmanagerv1.Certificate {
	return &certmanagerv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      CollectorName,
			Namespace: namespace,
		},
		Spec: certmanagerv1.CertificateSpec{
			SecretName: CollectorName + "-tls",
			IssuerRef: cmmeta.IssuerReference{
				Name: issuer.Name,
				Kind: "Issuer",
			},
			DNSNames: []string{
				CollectorName + "." + namespace + ".svc.cluster.local",
				CollectorName + "." + namespace + ".svc",
				CollectorName,
			},
		},
	}
}

// GetCollectorConfigMap returns a ConfigMap with the OTEL collector configuration.
// This configuration uses the Prometheus exporter to expose metrics on :9464/metrics
// and the debug exporter for traces.
func GetCollectorConfigMap(namespace string) *corev1.ConfigMap {
	config := `receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
        tls:
          cert_file: /etc/otel/certs/tls.crt
          key_file: /etc/otel/certs/tls.key
processors:
  batch:
    send_batch_size: 100
    timeout: 1s
exporters:
  debug:
    verbosity: detailed
  prometheus:
    endpoint: "0.0.0.0:9464"
    send_timestamps: true
    resource_to_telemetry_conversion:
      enabled: true
service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: [batch]
      exporters: [debug]
    metrics:
      receivers: [otlp]
      processors: [batch]
      exporters: [prometheus]
`

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      CollectorName + "-config",
			Namespace: namespace,
		},
		Data: map[string]string{
			"config.yaml": config,
		},
	}
}

// GetCollectorDeployment returns a Deployment for the OTEL collector.
func GetCollectorDeployment(namespace string) *appsv1.Deployment {
	labels := map[string]string{
		"app": CollectorName,
	}

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      CollectorName,
			Namespace: namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: new(int32(1)),
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: CollectorName,
					Containers: []corev1.Container{
						{
							Name:  "collector",
							Image: CollectorImage,
							Args: []string{
								"--config=/etc/otel/config.yaml",
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "otlp-grpc",
									ContainerPort: CollectorPort,
									Protocol:      corev1.ProtocolTCP,
								},
								{
									Name:          "metrics",
									ContainerPort: CollectorMetricsPort,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config",
									MountPath: "/etc/otel",
								},
								{
									Name:      "certs",
									MountPath: "/etc/otel/certs",
									ReadOnly:  true,
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: CollectorName + "-config",
									},
								},
							},
						},
						{
							Name: "certs",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: CollectorName + "-tls",
								},
							},
						},
					},
				},
			},
		},
	}
}

// GetCollectorService returns a Service for the OTEL collector.
func GetCollectorService(namespace string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      CollectorName,
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": CollectorName,
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "otlp-grpc",
					Port:       CollectorPort,
					TargetPort: intstr.FromInt32(CollectorPort),
					Protocol:   corev1.ProtocolTCP,
				},
				{
					Name:       "metrics",
					Port:       CollectorMetricsPort,
					TargetPort: intstr.FromInt32(CollectorMetricsPort),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}
}

// GetKlioServerOTELConfigMap returns a ConfigMap with OTEL configuration for the Klio server.
func GetKlioServerOTELConfigMap(namespace string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "klio-server-otel-config",
			Namespace: namespace,
		},
		Data: map[string]string{
			"OTEL_SERVICE_NAME":                     "klio-server",
			"OTEL_TRACES_EXPORTER":                  "otlp",
			"OTEL_METRICS_EXPORTER":                 "otlp",
			"OTEL_EXPORTER_OTLP_PROTOCOL":           "grpc",
			"OTEL_EXPORTER_OTLP_ENDPOINT":           "https://" + CollectorName + ":4317",
			"OTEL_EXPORTER_OTLP_INSECURE":           "false",
			"OTEL_EXPORTER_OTLP_CERTIFICATE":        "/otel/ca.crt",
			"OTEL_EXPORTER_OTLP_CLIENT_CERTIFICATE": "/otel/tls.crt",
			"OTEL_EXPORTER_OTLP_CLIENT_KEY":         "/otel/tls.key",
			"OTEL_METRIC_EXPORT_INTERVAL":           "5000",
			"OTEL_RESOURCE_DETECTORS":               "telemetry.sdk,host,os.type,process.executable.name",
			"OTEL_EXPORTER_OTLP_TRACES_COMPRESSION": "gzip",
			"OTEL_EXPORTER_OTLP_TRACES_TIMEOUT":     "10000",
			"OTEL_EXPORTER_OTLP_METRICS_TIMEOUT":    "10000",
		},
	}
}

// GetClusterOTELEnvVars returns the OTEL environment variables for the CNPG cluster sidecar
// as explicit corev1.EnvVar entries. These must be set via Cluster.Spec.Env (not EnvFrom)
// so that the Klio lifecycle webhook can merge them into the sidecar container.
func GetClusterOTELEnvVars() []corev1.EnvVar {
	return []corev1.EnvVar{
		{Name: "OTEL_SERVICE_NAME", Value: "klio-sidecar"},
		{Name: "OTEL_TRACES_EXPORTER", Value: "otlp"},
		{Name: "OTEL_METRICS_EXPORTER", Value: "otlp"},
		{Name: "OTEL_EXPORTER_OTLP_PROTOCOL", Value: "grpc"},
		{Name: "OTEL_EXPORTER_OTLP_ENDPOINT", Value: "https://" + CollectorName + ":4317"},
		{Name: "OTEL_EXPORTER_OTLP_INSECURE", Value: "false"},
		{Name: "OTEL_EXPORTER_OTLP_CERTIFICATE", Value: "/projected/otel-ca.crt"},
		{Name: "OTEL_EXPORTER_OTLP_CLIENT_CERTIFICATE", Value: "/projected/otel-tls.crt"},
		{Name: "OTEL_EXPORTER_OTLP_CLIENT_KEY", Value: "/projected/otel-tls.key"},
		{Name: "OTEL_METRIC_EXPORT_INTERVAL", Value: "5000"},
		{Name: "OTEL_RESOURCE_DETECTORS", Value: "telemetry.sdk,host,os.type,process.executable.name"},
		{Name: "OTEL_EXPORTER_OTLP_TRACES_COMPRESSION", Value: "gzip"},
		{Name: "OTEL_EXPORTER_OTLP_TRACES_TIMEOUT", Value: "10000"},
		{Name: "OTEL_EXPORTER_OTLP_METRICS_TIMEOUT", Value: "10000"},
	}
}

// GetOTELClientCertificate returns a client Certificate for OTEL authentication.
func GetOTELClientCertificate(
	name, namespace string,
	issuer *certmanagerv1.Issuer,
) *certmanagerv1.Certificate {
	return &certmanagerv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: certmanagerv1.CertificateSpec{
			SecretName: name,
			IssuerRef: cmmeta.IssuerReference{
				Name: issuer.Name,
				Kind: "Issuer",
			},
			CommonName: "otel-client",
			Usages: []certmanagerv1.KeyUsage{
				certmanagerv1.UsageClientAuth,
			},
		},
	}
}
