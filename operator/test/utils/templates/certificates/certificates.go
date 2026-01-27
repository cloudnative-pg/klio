package certificates

import (
	"fmt"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetSelfSignedIssuerObject returns a self-signed issuer object.
func GetSelfSignedIssuerObject(name, namespace string) *certmanagerv1.Issuer {
	return &certmanagerv1.Issuer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: certmanagerv1.IssuerSpec{
			IssuerConfig: certmanagerv1.IssuerConfig{
				SelfSigned: &certmanagerv1.SelfSignedIssuer{},
			},
		},
	}
}

// GetCACertificateObject returns a Certificate creating a CA.
func GetCACertificateObject(
	name, namespace string,
	issuer *certmanagerv1.Issuer,
) *certmanagerv1.Certificate {
	return &certmanagerv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: certmanagerv1.CertificateSpec{
			CommonName: name,
			SecretName: name,
			IssuerRef: cmmeta.ObjectReference{
				Name:  issuer.Name,
				Kind:  issuer.Kind,
				Group: issuer.GroupVersionKind().Group,
			},
			IsCA: true,
			Usages: []certmanagerv1.KeyUsage{
				certmanagerv1.UsageCertSign,
			},
		},
	}
}

// GetUserCertificateObject gets a certificate creating the secret to
// authenticate a Klio user.
func GetUserCertificateObject(
	name, namespace string,
	username string,
	issuer *certmanagerv1.Issuer,
) *certmanagerv1.Certificate {
	return &certmanagerv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: certmanagerv1.CertificateSpec{
			CommonName: username,
			SecretName: name + "-auth",
			IssuerRef: cmmeta.ObjectReference{
				Name:  issuer.Name,
				Kind:  issuer.Kind,
				Group: issuer.GroupVersionKind().Group,
			},
			IsCA: false,
			Usages: []certmanagerv1.KeyUsage{
				certmanagerv1.UsageClientAuth,
			},
		},
	}
}

// GetCAIssuerObject returns a Issuer that signs the generate certificates
// using the passed secret name.
func GetCAIssuerObject(name, namespace, secretName string) *certmanagerv1.Issuer {
	return &certmanagerv1.Issuer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: certmanagerv1.IssuerSpec{
			IssuerConfig: certmanagerv1.IssuerConfig{
				CA: &certmanagerv1.CAIssuer{
					SecretName: secretName,
				},
			},
		},
	}
}

// GetCertificateObject returns Certificate object.
func GetCertificateObject(
	name, namespace string,
	dnsNames []string,
	issuer *certmanagerv1.Issuer,
) *certmanagerv1.Certificate {
	requestedDNS := make([]string, 0, len(dnsNames))
	for _, dns := range dnsNames {
		requestedDNS = append(requestedDNS, fmt.Sprintf("%s.%s", dns, namespace))
	}

	return &certmanagerv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: certmanagerv1.CertificateSpec{
			CommonName: name,
			DNSNames:   requestedDNS,
			SecretName: name + "-tls",
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
