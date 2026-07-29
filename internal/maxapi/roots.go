package maxapi

import (
	"crypto/x509"
	_ "embed"
	"fmt"
)

// russianTrustedCA is the official Russian Trusted CA bundle published by the
// Ministry of Digital Development at https://gu-st.ru/content/Other/doc/russiantrustedca.pem.
//
// platform-api2.max.ru presents a chain issued by "Russian Trusted Sub CA",
// which no mainstream CA store contains, so TLS verification fails without it.
// The bundle is vendored (never fetched at runtime) and added only to this
// client's root pool, leaving the system trust store untouched.
//
//go:embed certs/russian_trusted_ca.pem
var russianTrustedCA []byte

// rootPool returns the system roots extended with the MAX certificate chain.
func rootPool() (*x509.CertPool, error) {
	pool, err := x509.SystemCertPool()
	if err != nil {
		pool = x509.NewCertPool()
	}
	if !pool.AppendCertsFromPEM(russianTrustedCA) {
		return nil, fmt.Errorf("embedded Russian Trusted CA bundle is not valid PEM")
	}
	return pool, nil
}
