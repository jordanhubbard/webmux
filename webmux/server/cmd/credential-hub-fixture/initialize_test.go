package main

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
)

func TestSyntheticCertificatesBindActualAuthority(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "wmx-cred-pki")
	if e := initializeFixtures(root, parent, parent, "hub=127.0.0.1,receiver=127.0.0.1"); e != nil {
		t.Fatal(e)
	}
	certificate := func(name string) *x509.Certificate {
		t.Helper()
		data, e := os.ReadFile(filepath.Join(root, name))
		if e != nil {
			t.Fatal(e)
		}
		block, _ := pem.Decode(data)
		if block == nil {
			t.Fatal("missing fixture certificate")
		}
		c, e := x509.ParseCertificate(block.Bytes)
		if e != nil {
			t.Fatal(e)
		}
		return c
	}
	ca := certificate("ca.pem")
	pool := x509.NewCertPool()
	pool.AddCert(ca)
	for _, role := range []string{"hub", "receiver"} {
		leaf := certificate(filepath.Join(role, "tls.pem"))
		if len(ca.SubjectKeyId) == 0 || !bytes.Equal(leaf.AuthorityKeyId, ca.SubjectKeyId) {
			t.Fatal("strict TLS authority identifier missing or mismatched")
		}
		if _, e := leaf.Verify(x509.VerifyOptions{Roots: pool, DNSName: "127.0.0.1", KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}); e != nil {
			t.Fatal(e)
		}
	}
}
