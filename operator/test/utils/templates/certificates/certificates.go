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
			IssuerRef: cmmeta.IssuerReference{
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
			IssuerRef: cmmeta.IssuerReference{
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
