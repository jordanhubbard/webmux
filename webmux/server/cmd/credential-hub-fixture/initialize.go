package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

func fixtureRandom(n int) string {
	b := make([]byte, n)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
func initializeFixtures(root, defaults, web, hosts string) error {
	if !filepath.IsAbs(root) || !strings.HasPrefix(filepath.Base(root), "wmx-cred-") || defaults == "" || web == "" {
		return errors.New("a new marked fixture root and public assets are required")
	}
	roles := strings.Split(hosts, ",")
	if len(roles) < 2 || len(roles) > 3 {
		return errors.New("two or three roles required")
	}
	names := map[string]bool{}
	addresses := map[string]string{}
	for _, entry := range roles {
		pair := strings.SplitN(entry, "=", 2)
		if len(pair) != 2 || !regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`).MatchString(pair[0]) || net.ParseIP(pair[1]) == nil || names[pair[0]] {
			return errors.New("invalid fixture role/IP")
		}
		names[pair[0]] = true
		addresses[pair[0]] = pair[1]
	}
	if e := os.Mkdir(root, 0700); e != nil {
		return e
	}
	key, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		return e
	}
	pub := &key.PublicKey
	now := time.Now()
	caTemplate := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "WebMux synthetic credential qualification CA"}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(2 * time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	caDER, e := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, pub, key)
	if e != nil {
		return e
	}
	// Parse the actual root, including its generated SubjectKeyId. Reusing the
	// unpopulated template omits leaf AuthorityKeyId and strict TLS rejects it.
	caCertificate, e := x509.ParseCertificate(caDER)
	if e != nil {
		return e
	}
	caPath := filepath.Join(root, "ca.pem")
	if e = os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}), 0600); e != nil {
		return e
	}
	fakeKeys := []string{"wmx-canary-" + fixtureRandom(32), "wmx-canary-" + fixtureRandom(32)}
	manifest := struct {
		Version int               `json:"version"`
		CA      string            `json:"ca"`
		Nodes   map[string]config `json:"nodes"`
	}{Version: 1, CA: caPath, Nodes: map[string]config{}}
	for i, entry := range roles {
		name := strings.SplitN(entry, "=", 2)[0]
		dir := filepath.Join(root, name)
		if e = os.Mkdir(dir, 0700); e != nil {
			return e
		}
		home := filepath.Join(dir, "wmx-cred-"+name)
		if e = os.Mkdir(home, 0700); e != nil {
			return e
		}
		if e = os.WriteFile(filepath.Join(home, ".webmux-credential-fixture"), []byte("webmux-credential-qualification-v1\n"), 0600); e != nil {
			return e
		}
		nodeKey, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if e != nil {
			return e
		}
		nodePub := &nodeKey.PublicKey
		certTemplate := &x509.Certificate{SerialNumber: big.NewInt(int64(i + 2)), Subject: pkix.Name{CommonName: "WebMux synthetic " + name}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(2 * time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, DNSNames: []string{"localhost"}, IPAddresses: []net.IP{net.ParseIP(addresses[name]), net.ParseIP("127.0.0.1"), net.ParseIP("::1")}}
		der, e := x509.CreateCertificate(rand.Reader, certTemplate, caCertificate, nodePub, key)
		if e != nil {
			return e
		}
		private, e := x509.MarshalPKCS8PrivateKey(nodeKey)
		if e != nil {
			return e
		}
		certPath, keyPath := filepath.Join(dir, "tls.pem"), filepath.Join(dir, "tls.key")
		if e = os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600); e != nil {
			return e
		}
		if e = os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private}), 0600); e != nil {
			return e
		}
		c := config{TestOnly: true, Home: home, Defaults: defaults, Web: web, Listen: net.JoinHostPort(addresses[name], "0"), Certificate: certPath, Key: keyPath, CA: caPath, NodeID: fixtureRandom(16), FakeKeys: fakeKeys, ReadyFile: filepath.Join(dir, "ready.json"), SessionSecret: "wmx-canary-" + fixtureRandom(32), AccountPassword: "wmx-canary-" + fixtureRandom(32)}
		data, e := json.MarshalIndent(c, "", "  ")
		if e != nil {
			return e
		}
		if e = os.WriteFile(filepath.Join(dir, "config.json"), append(data, '\n'), 0600); e != nil {
			return e
		}
		manifest.Nodes[name] = c
	}
	data, e := json.MarshalIndent(manifest, "", "  ")
	if e != nil {
		return e
	}
	return os.WriteFile(filepath.Join(root, "manifest.json"), append(data, '\n'), 0600)
}
