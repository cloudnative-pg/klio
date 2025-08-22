package secrets

import (
	"fmt"

	"github.com/foomo/htpasswd"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetKlioClientSecret returns a Klio client secret object.
func GetKlioClientSecret(name, namespace, username, password string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"username": []byte(username),
			"password": []byte(password),
		},
	}
}

// GetKlioUsersSecret returns a Klio users secret object.
func GetKlioUsersSecret(
	name, namespace string,
	clientSecret *corev1.Secret,
	clusterName string,
) *corev1.Secret {
	hp := htpasswd.HashedPasswords{}
	_ = hp.SetPassword(
		fmt.Sprintf("%v@%v", string(clientSecret.Data["username"]), clusterName),
		string(clientSecret.Data["password"]),
		htpasswd.HashBCrypt,
	)

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Immutable: nil,
		Data: map[string][]byte{
			"htpasswd": hp.Bytes(),
		},
	}
}

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
		Immutable: nil,
		Data: map[string][]byte{
			"password": []byte(encryptionPassword),
		},
	}
}
