package rustfs

import (
	"fmt"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
)

const (
	// RustFSImage is the RustFS container image.
	//nolint:godot
	// renovate image: datasource=docker depName=rustfs/rustfs versioning=docker
	RustFSImage = "rustfs/rustfs:1.0.0-alpha.83@sha256:1cfa82fb394a7c6a4ffc24f5333b52afee608d258503f94b37fcce5cc965c896"

	// BusyboxImage is the busybox container image used for init containers.
	//nolint:godot
	// renovate image: datasource=docker depName=busybox versioning=docker
	BusyboxImage = "busybox:1.37.0@sha256:b3255e7dfbcd10cb367af0d409747d511aeb66dfac98cf30e97e87e4207dd76f"

	// AWSCLIImage is the AWS CLI container image used for S3 operations.
	//nolint:godot
	// renovate image: datasource=docker depName=amazon/aws-cli versioning=docker
	AWSCLIImage = "amazon/aws-cli:2.34.12@sha256:062e71cf5cdeb9d6836465823376726bdb6fac358ccb3e8d9106de48f51f7e68"

	// RustFSAccessKey is the access key for RustFS.
	RustFSAccessKey = "rustfsaccesskey1234567890"

	// RustFSSecretKey is the secret key for RustFS.
	RustFSSecretKey = "rustfssecretkeyabcdefghijklmnop"

	// RustFSBucketName is the default bucket name.
	RustFSBucketName = "bucket"

	// RustFSRegion is the default region.
	RustFSRegion = "us-east-1"

	// Kubernetes label keys.
	labelName     = "app.kubernetes.io/name"
	labelInstance = "app.kubernetes.io/instance"

	// Secret and resource names.
	rustfsSecretName = "rustfs-secret"
	rustfsTLSSecret  = "rustfs-tls" //nolint:gosec
)

// GetRustFSSecret returns a Secret with RustFS credentials.
func GetRustFSSecret(name, namespace string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"RUSTFS_ACCESS_KEY": []byte(RustFSAccessKey),
			"RUSTFS_SECRET_KEY": []byte(RustFSSecretKey),
		},
	}
}

// GetRustFSConfigMap returns a ConfigMap with RustFS environment configuration.
func GetRustFSConfigMap(name, namespace string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string]string{
			"RUSTFS_ADDRESS":           ":9000",
			"RUSTFS_CONSOLE_ADDRESS":   ":9001",
			"RUSTFS_REGION":            RustFSRegion,
			"RUSTFS_VOLUMES":           "/data",
			"RUSTFS_TLS_PATH":          "/certs",
			"RUSTFS_CONSOLE_ENABLE":    "true",
			"RUSTFS_OBS_ENVIRONMENT":   "develop",
			"RUSTFS_OBS_LOG_DIRECTORY": "/logs",
			"RUSTFS_OBS_LOGGER_LEVEL":  "debug",
		},
	}
}

// GetRustFSPVC returns a PersistentVolumeClaim for RustFS data storage.
func GetRustFSPVC(name, namespace string) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("1Gi"),
				},
			},
		},
	}
}

// GetRustFSLogsPVC returns a PersistentVolumeClaim for RustFS logs.
func GetRustFSLogsPVC(name, namespace string) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("1Gi"),
				},
			},
		},
	}
}

// GetRustFSCertificate returns a Certificate for RustFS TLS.
func GetRustFSCertificate(name, namespace string, issuer *certmanagerv1.Issuer) *certmanagerv1.Certificate {
	return &certmanagerv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: certmanagerv1.CertificateSpec{
			CommonName: "rustfs",
			DNSNames: []string{
				"rustfs",
				"rustfs." + namespace,
				"rustfs." + namespace + ".svc",
			},
			SecretName: rustfsTLSSecret,
			IssuerRef: cmmeta.IssuerReference{
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
}

// GetRustFSService returns a Service exposing RustFS.
func GetRustFSService(name, namespace string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				labelName:     name,
				labelInstance: name,
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "api",
					Port:       9000,
					TargetPort: intstr.FromInt32(9000),
					Protocol:   corev1.ProtocolTCP,
				},
				{
					Name:       "console",
					Port:       9001,
					TargetPort: intstr.FromInt32(9001),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}
}

// GetRustFSDeployment returns a Deployment for RustFS.
func GetRustFSDeployment(name, namespace string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				labelName:     name,
				labelInstance: name,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(int32(1)),
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RollingUpdateDeploymentStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDeployment{
					MaxUnavailable: ptr.To(intstr.FromInt32(1)),
					MaxSurge:       ptr.To(intstr.FromInt32(0)),
				},
			},
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					labelName:     name,
					labelInstance: name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						labelName:     name,
						labelInstance: name,
					},
				},
				Spec: corev1.PodSpec{
					SecurityContext: &corev1.PodSecurityContext{
						FSGroup:    ptr.To(int64(10001)),
						RunAsGroup: ptr.To(int64(10001)),
						RunAsUser:  ptr.To(int64(10001)),
					},
					InitContainers: []corev1.Container{
						{
							Name:  "init-step",
							Image: BusyboxImage,
							Command: []string{
								"sh",
								"-c",
								"mkdir -p /data /logs && chown 10001:10001 /data /logs",
							},
							SecurityContext: &corev1.SecurityContext{
								RunAsUser:  ptr.To(int64(0)),
								RunAsGroup: ptr.To(int64(0)),
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "data",
									MountPath: "/data",
								},
								{
									Name:      "logs",
									MountPath: "/logs",
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:  "rustfs",
							Image: RustFSImage,
							Command: []string{
								"/usr/bin/rustfs",
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "endpoint",
									ContainerPort: 9000,
									Protocol:      corev1.ProtocolTCP,
								},
								{
									Name:          "console",
									ContainerPort: 9001,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							EnvFrom: []corev1.EnvFromSource{
								{
									ConfigMapRef: &corev1.ConfigMapEnvSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: name + "-config",
										},
									},
								},
								{
									SecretRef: &corev1.SecretEnvSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: rustfsSecretName,
										},
									},
								},
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path:   "/health",
										Port:   intstr.FromInt32(9000),
										Scheme: corev1.URISchemeHTTPS,
									},
								},
								InitialDelaySeconds: 10,
								PeriodSeconds:       10,
								TimeoutSeconds:      5,
								SuccessThreshold:    1,
								FailureThreshold:    3,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path:   "/health",
										Port:   intstr.FromInt32(9000),
										Scheme: corev1.URISchemeHTTPS,
									},
								},
								InitialDelaySeconds: 5,
								PeriodSeconds:       5,
								TimeoutSeconds:      3,
								SuccessThreshold:    1,
								FailureThreshold:    3,
							},
							SecurityContext: &corev1.SecurityContext{
								RunAsNonRoot:           ptr.To(true),
								ReadOnlyRootFilesystem: ptr.To(true),
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{"ALL"},
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "data",
									MountPath: "/data",
								},
								{
									Name:      "logs",
									MountPath: "/logs",
								},
								{
									Name:      "tmp",
									MountPath: "/tmp",
								},
								{
									Name:      "certs",
									MountPath: "/certs/rustfs_cert.pem",
									SubPath:   "rustfs_cert.pem",
								},
								{
									Name:      "certs",
									MountPath: "/certs/rustfs_key.pem",
									SubPath:   "rustfs_key.pem",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "data",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: name + "-data",
								},
							},
						},
						{
							Name: "logs",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: name + "-logs",
								},
							},
						},
						{
							Name: "tmp",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
						{
							Name: "certs",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: rustfsTLSSecret,
									Items: []corev1.KeyToPath{
										{
											Key:  "tls.crt",
											Path: "rustfs_cert.pem",
										},
										{
											Key:  "tls.key",
											Path: "rustfs_key.pem",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

// GetRustFSCreateBucketJob returns a Job to create an S3 bucket in RustFS.
func GetRustFSCreateBucketJob(name, namespace, bucketName string) *batchv1.Job {
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyOnFailure,
					Containers: []corev1.Container{
						{
							Name:  "create-bucket",
							Image: AWSCLIImage,
							Command: []string{
								"/bin/sh",
								"-c",
							},
							Args: []string{
								fmt.Sprintf("aws s3 mb s3://%v --endpoint-url=%s",
									bucketName,
									GetRustFSEndpoint("rustfs", namespace),
								),
							},
							Env: []corev1.EnvVar{
								{
									Name: "AWS_ACCESS_KEY_ID",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: rustfsSecretName,
											},
											Key: "RUSTFS_ACCESS_KEY",
										},
									},
								},
								{
									Name: "AWS_SECRET_ACCESS_KEY",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: rustfsSecretName,
											},
											Key: "RUSTFS_SECRET_KEY",
										},
									},
								},
								{
									Name:  "AWS_CA_BUNDLE",
									Value: "/certs/ca.crt",
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "certs",
									MountPath: "/certs/ca.crt",
									SubPath:   "ca.crt",
									ReadOnly:  true,
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "certs",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: rustfsTLSSecret,
									Items: []corev1.KeyToPath{
										{
											Key:  "tls.crt",
											Path: "ca.crt",
										},
									},
								},
							},
						},
					},
				},
			},
			BackoffLimit: ptr.To(int32(4)),
		},
	}
}

// GetRustFSEndpoint returns the RustFS endpoint URL.
func GetRustFSEndpoint(serviceName, namespace string) string {
	return fmt.Sprintf("https://%s.%s.svc:9000", serviceName, namespace)
}
