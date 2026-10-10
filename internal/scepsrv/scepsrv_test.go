package scepsrv

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"log/slog"
	"math/big"
	"os"
	"sync"
	"testing"
	"time"
)

type memDepot struct {
	mu        sync.Mutex
	cert, key string
	serial    int64
	issued    int
}

func (m *memDepot) GetSCEPCA(context.Context) (string, string, error) {
	if m.cert == "" {
		return "", "", os.ErrNotExist
	}
	return m.cert, m.key, nil
}
func (m *memDepot) SaveSCEPCA(_ context.Context, c, k string) error { m.cert, m.key = c, k; return nil }
func (m *memDepot) NextSCEPSerial(context.Context) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.serial++
	return m.serial, nil
}
func (m *memDepot) PutSCEPCert(context.Context, string, string, string, time.Time, time.Time) error {
	m.issued++
	return nil
}

func TestGenerateAndVerify(t *testing.T) {
	d := &memDepot{}
	s, err := New(context.Background(), d, func(context.Context) string { return "chal" }, "Test", slog.New(slog.NewTextHandler(os.Stderr, nil)))
	if err != nil {
		t.Fatal(err)
	}
	if d.cert == "" {
		t.Fatal("CA not persisted")
	}
	// Reload from depot must yield the same CA.
	s2, err := New(context.Background(), d, nil, "Test", slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	if !s2.CACert().Equal(s.CACert()) {
		t.Fatal("CA changed on reload")
	}
	// A certificate signed by the CA verifies; a self-signed one does not.
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(5), Subject: pkix.Name{CommonName: "dev"}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}
	der, _ := x509.CreateCertificate(rand.Reader, tmpl, s.CACert(), &key.PublicKey, s.key)
	leaf, _ := x509.ParseCertificate(der)
	if err := s.Verify(context.Background(), leaf); err != nil {
		t.Fatalf("verify: %v", err)
	}
	selfDER, _ := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	self, _ := x509.ParseCertificate(selfDER)
	if err := s.Verify(context.Background(), self); err == nil {
		t.Fatal("self-signed cert should not verify")
	}
}
