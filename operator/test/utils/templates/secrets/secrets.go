package secrets

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetKlioEncryptionSecret returns a Klio encryption secret object.
func GetKlioEncryptionSecret(
	name, namespace,
	encryptionPassword string,
) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"password": []byte(encryptionPassword),
		},
	}
}

// GetS3CredentialsSecret returns an S3 credentials secret object.
func GetS3CredentialsSecret(
	name, namespace,
	accessKey, secretKey string,
) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"RUSTFS_ACCESS_KEY": []byte(accessKey),
			"RUSTFS_SECRET_KEY": []byte(secretKey),
		},
	}
}
