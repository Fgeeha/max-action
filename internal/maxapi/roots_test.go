package maxapi

import (
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"
)

// caRenewalLead is how long before a vendored certificate expires the tests
// start failing. Re-vendoring means locating the current official bundle and
// re-checking its checksum, so the warning has to arrive well in advance.
const caRenewalLead = 90 * 24 * time.Hour

// TestEmbeddedCANotExpiring fails once any vendored certificate is close to
// its expiry date. Without it the bundle would go stale silently and every
// consumer of the action would start failing the TLS handshake at once.
func TestEmbeddedCANotExpiring(t *testing.T) {
	certs := parseEmbeddedCA(t)
	deadline := time.Now().Add(caRenewalLead)

	for _, cert := range certs {
		if cert.NotAfter.Before(deadline) {
			t.Errorf("vendored certificate %q expires %s, which is less than %d days away: "+
				"refresh internal/maxapi/certs/russian_trusted_ca.pem from the official source",
				cert.Subject.CommonName, cert.NotAfter.Format(time.DateOnly), int(caRenewalLead.Hours()/24))
		}
		if cert.NotBefore.After(time.Now()) {
			t.Errorf("vendored certificate %q is not valid until %s",
				cert.Subject.CommonName, cert.NotBefore.Format(time.DateOnly))
		}
	}
}

// TestEmbeddedCAContainsOnlyCAs checks the bundle still holds the MAX chain
// and nothing that does not belong in a root pool. Loading it into a pool is
// covered by TestEmbeddedCAIsLoadable.
func TestEmbeddedCAContainsOnlyCAs(t *testing.T) {
	certs := parseEmbeddedCA(t)
	if len(certs) < 2 {
		t.Fatalf("expected the root and sub CA in the bundle, got %d certificates", len(certs))
	}
	for _, cert := range certs {
		if !cert.IsCA {
			t.Errorf("certificate %q is not a CA and does not belong in a root bundle", cert.Subject.CommonName)
		}
	}
}

func parseEmbeddedCA(t *testing.T) []*x509.Certificate {
	t.Helper()

	var certs []*x509.Certificate
	rest := russianTrustedCA
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			t.Fatalf("cannot parse a certificate from the embedded bundle: %v", err)
		}
		certs = append(certs, cert)
	}

	if len(certs) == 0 {
		t.Fatal("the embedded bundle contains no certificates")
	}
	return certs
}
