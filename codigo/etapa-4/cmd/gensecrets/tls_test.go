package main

import (
	"bytes"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"testing"
	"time"
)

func TestChainNamesItsCA(t *testing.T) {
	files := tlsChain("demo.example")
	ca, err := parsePEMCert(files["tls_ca.pem"])
	if err != nil {
		t.Fatal(err)
	}
	crt, err := parsePEMCert(files["tls.crt"])
	if err != nil {
		t.Fatal(err)
	}
	if len(ca.SubjectKeyId) == 0 || len(crt.SubjectKeyId) == 0 || !bytes.Equal(crt.AuthorityKeyId, ca.SubjectKeyId) {
		t.Fatalf("key identifiers: CA SKI %x, nginx SKI %x, nginx AKI %x", ca.SubjectKeyId, crt.SubjectKeyId, crt.AuthorityKeyId)
	}
	pool := x509.NewCertPool()
	pool.AddCert(ca)
	for _, name := range []string{"demo.example", "localhost", "nginx", "127.0.0.1"} {
		if _, err := crt.Verify(x509.VerifyOptions{Roots: pool, DNSName: name}); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	if !tlsCurrent(files["tls_ca.pem"], files["tls.crt"], "demo.example", time.Now()) {
		t.Error("a fresh chain is not current")
	}
}

func TestChainRenewedWhenItNoLongerFits(t *testing.T) {
	files := tlsChain("localhost")
	if tlsCurrent(files["tls_ca.pem"], files["tls.crt"], "other.example", time.Now()) {
		t.Error("a chain for another host is current")
	}
	if tlsCurrent(files["tls_ca.pem"], files["tls.crt"], "localhost", time.Now().AddDate(0, 11, 25)) {
		t.Error("a chain 10 days from expiring is current")
	}

	// The old chain: signed with the CA template, so without AKI.
	caKey, key := newECKey(), newECKey()
	caTmpl := &x509.Certificate{SerialNumber: serial(), Subject: pkix.Name{CommonName: "old CA"}, IsCA: true,
		BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().AddDate(1, 0, 0)}
	caDER, _ := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	leaf := &x509.Certificate{SerialNumber: serial(), DNSNames: []string{"localhost"}, NotBefore: time.Now().Add(-time.Hour),
		NotAfter: time.Now().AddDate(1, 0, 0), ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	leafDER, _ := x509.CreateCertificate(rand.Reader, leaf, caTmpl, &key.PublicKey, caKey)
	old := func(der []byte) []byte { return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}) }
	if tlsCurrent(old(caDER), old(leafDER), "localhost", time.Now()) {
		t.Error("a chain without AKI is current")
	}
}
