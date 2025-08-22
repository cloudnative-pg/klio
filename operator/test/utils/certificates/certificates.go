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
}
