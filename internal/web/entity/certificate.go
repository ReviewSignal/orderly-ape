// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package entity

import (
	"crypto/x509"
	"encoding/pem"
	"net/url"
	"time"

	"github.com/a-h/templ"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Certificate struct {
	*corev1.Secret

	cert *x509.Certificate
}

type CertificateList []Certificate

func WrapSecret(secret *corev1.Secret) Certificate {
	return Certificate{Secret: secret}
}

func WrapSecretList(items []corev1.Secret) CertificateList {
	wrapped := make(CertificateList, len(items))
	for i, item := range items {
		wrapped[i] = WrapSecret(&item)
	}
	return wrapped
}

func (p Certificate) NamespacedName() string {
	return client.ObjectKeyFromObject(p.Secret).String()
}

func (p Certificate) ObjectLink() templ.SafeURL {
	path, _ := url.JoinPath("projects", p.Namespace, "certificates", p.Name)
	return templ.URL(path)
}

func (p Certificate) VerboseName() string {
	return "Certificate"
}

func (p Certificate) Cert() x509.Certificate {
	if p.Secret == nil || p.Type != corev1.SecretTypeTLS {
		return x509.Certificate{}
	}

	if p.cert == nil {
		block, _ := pem.Decode(p.Data["tls.crt"])
		if block == nil {
			return x509.Certificate{}
		}

		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return x509.Certificate{}
		}
		p.cert = cert
	}
	return *p.cert
}

func (p Certificate) IssuerName() string {
	cert := p.Cert()
	if len(cert.Issuer.Organization) > 0 {
		return cert.Issuer.Organization[0] + " (" + cert.Issuer.CommonName + ")"
	}

	return cert.Issuer.CommonName
}

func (p Certificate) ExpirationDate() string {
	cert := p.Cert()
	if cert.NotAfter.IsZero() {
		return "Unknown"
	}
	return cert.NotAfter.Format("2006-01-02")
}

func removeDuplicates(input []string) []string {
	seen := make(map[string]struct{})
	result := []string{}
	for _, v := range input {
		if _, ok := seen[v]; !ok {
			seen[v] = struct{}{}
			result = append(result, v)
		}
	}
	return result
}

func (p Certificate) DNSNames() []string {
	cert := p.Cert()
	names := make([]string, 0, len(cert.DNSNames)+1)
	names = append(names, cert.Subject.CommonName)
	names = append(names, cert.DNSNames...)
	return removeDuplicates(names)
}

func (p Certificate) ExpiresIn() time.Duration {
	return time.Until(p.Cert().NotAfter)
}

func (p Certificate) IsExpired() bool {
	return p.ExpiresIn() <= 0
}
