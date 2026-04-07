package secrets

import (
	"bytes"
	"fmt"
	"io"

	"filippo.io/age"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AgeEncryptionSecrets holds both secrets needed for Age-based encryption.
type AgeEncryptionSecrets struct {
	// EncryptionKeySecret contains the Age-encrypted key file.
	EncryptionKeySecret *corev1.Secret
	// IdentitySecret contains the Age identity (private key) file.
	IdentitySecret *corev1.Secret
}

// GetKlioAgeEncryptionSecrets generates an Age keypair, encrypts the
// password, and returns two Kubernetes secrets:
//   - {name} with key "encryption-key.age" containing the Age-encrypted password
//   - {name}-identity with key "identity.txt" containing the Age identity.
//
// Panics on cryptographic errors (only possible if the system RNG is broken).
func GetKlioAgeEncryptionSecrets(
	name, namespace, encryptionPassword string,
) *AgeEncryptionSecrets {
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		panic(fmt.Sprintf("generating Age identity: %v", err))
	}

	var encrypted bytes.Buffer
	writer, err := age.Encrypt(&encrypted, identity.Recipient())
	if err != nil {
		panic(fmt.Sprintf("creating Age encryptor: %v", err))
	}
	if _, err := io.WriteString(writer, encryptionPassword); err != nil {
		panic(fmt.Sprintf("writing plaintext to Age encryptor: %v", err))
	}
	if err := writer.Close(); err != nil {
		panic(fmt.Sprintf("closing Age encryptor: %v", err))
	}

	identityContent := fmt.Sprintf(
		"# public key: %s\n%s\n",
		identity.Recipient(), identity,
	)

	return &AgeEncryptionSecrets{
		EncryptionKeySecret: &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Data: map[string][]byte{
				"encryption-key.age": encrypted.Bytes(),
			},
		},
		IdentitySecret: &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name + "-identity",
				Namespace: namespace,
			},
			Data: map[string][]byte{
				"identity.txt": []byte(identityContent),
			},
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
