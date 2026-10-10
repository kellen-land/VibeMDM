package pushcert

import (
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"testing"

	"github.com/smallstep/pkcs7"
)

// Simulates mdmcert.download: encrypt a payload to the request's encryption
// certificate, hex-encode it (as the service does), then decrypt.
func TestRequestRoundTrip(t *testing.T) {
	req, err := NewRequest("Acme", "it@acme.example")
	if err != nil {
		t.Fatal(err)
	}
	blk, _ := pem.Decode([]byte(req.CSRPEM))
	csr, err := x509.ParseCertificateRequest(blk.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if err := csr.CheckSignature(); err != nil {
		t.Fatal(err)
	}
	if csr.Subject.CommonName != "Acme MDM Push" || len(csr.EmailAddresses) != 1 {
		t.Fatalf("unexpected CSR subject: %+v", csr.Subject)
	}
	cb, _ := pem.Decode([]byte(req.CertPEM))
	encCert, _ := x509.ParseCertificate(cb.Bytes)
	payload := []byte(`<?xml version="1.0"?><plist version="1.0"><dict><key>PushCertRequestCSR</key><string>abc</string></dict></plist>`)
	enc, err := pkcs7.Encrypt(payload, []*x509.Certificate{encCert})
	if err != nil {
		t.Fatal(err)
	}
	for name, in := range map[string][]byte{"hex": []byte(hex.EncodeToString(enc) + "\n"), "der": enc} {
		out, err := DecryptSignedRequest(in, req)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if string(out) != string(payload) {
			t.Fatalf("%s: payload mismatch", name)
		}
	}
	// Wrong key material must fail.
	other, _ := NewRequest("Other", "")
	if _, err := DecryptSignedRequest(enc, other); err == nil {
		t.Fatal("decryption with the wrong key should fail")
	}
}

func TestNormalizeKeyAndMatch(t *testing.T) {
	req, _ := NewRequest("Acme", "")
	keyPEM, key, err := NormalizeKeyPEM([]byte(req.KeyPEM))
	if err != nil || keyPEM == "" {
		t.Fatal(err)
	}
	certPEM, cert, err := NormalizeCertificatePEM([]byte(req.CertPEM))
	if err != nil || certPEM == "" {
		t.Fatal(err)
	}
	if !KeyMatchesCert(cert, key) {
		t.Fatal("key should match cert")
	}
	if _, err := TopicFromCert(cert); err == nil {
		t.Fatal("encryption cert has no UID; expected error")
	}
}
