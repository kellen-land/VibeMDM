package agentinstall

import (
	"bytes"
	"compress/zlib"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/binary"
	"encoding/pem"
	"math/big"
	"strings"
	"testing"
	"time"

	"howett.net/plist"
)

func csrPEM(t *testing.T, bits int) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: "Agent Signing"}}, key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der})
}

func TestCAIssuesCodeSigningCertificates(t *testing.T) {
	caPEM, keyPEM, err := NewCA("Acme")
	if err != nil {
		t.Fatal(err)
	}
	ca, _, err := parseCA(caPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	if !ca.IsCA || ca.MaxPathLen != 0 || !ca.MaxPathLenZero || len(ca.PermittedDNSDomains) != 1 {
		t.Fatalf("CA constraints missing: %+v", ca)
	}
	chain, err := SignCSR(csrPEM(t, 2048), caPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	blk, rest := pem.Decode([]byte(chain))
	leaf, err := x509.ParseCertificate(blk.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rest), "BEGIN CERTIFICATE") {
		t.Fatal("chain must include the CA")
	}
	pool := x509.NewCertPool()
	pool.AddCert(ca)
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: pool, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny}}); err != nil {
		t.Fatalf("leaf does not chain to the CA: %v", err)
	}
	// productsign wants Apple's installer-signing usage and no other.
	if len(leaf.ExtKeyUsage) != 0 || len(leaf.UnknownExtKeyUsage) != 1 || !leaf.UnknownExtKeyUsage[0].Equal(OIDInstallerSigning) {
		t.Fatalf("leaf usages: %v %v", leaf.ExtKeyUsage, leaf.UnknownExtKeyUsage)
	}
	// The root must not vouch for a TLS server.
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: pool, DNSName: "mdm.example.com"}); err == nil {
		t.Fatal("installer-signing leaf verified for TLS")
	}
	if !CAIsCurrent(caPEM) {
		t.Fatal("a new CA must be current")
	}
	if _, err := SignCSR(csrPEM(t, 1024), caPEM, keyPEM); err == nil {
		t.Fatal("weak key accepted")
	}
	if _, err := SignCSR([]byte("junk"), caPEM, keyPEM); err == nil {
		t.Fatal("junk accepted")
	}
	if Fingerprint(caPEM) == "" {
		t.Fatal("fingerprint")
	}
}

func TestBootstrapProfileAndManifest(t *testing.T) {
	caPEM, _, _ := NewCA("Acme")
	b, err := BootstrapProfile("Acme", caPEM, "https://mdm.example.com/", "vmbt_secret")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if _, err := plist.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	if doc["PayloadIdentifier"] != BootstrapIdentifier {
		t.Fatalf("identifier %v", doc["PayloadIdentifier"])
	}
	content := doc["PayloadContent"].([]any)
	types := []string{content[0].(map[string]any)["PayloadType"].(string), content[1].(map[string]any)["PayloadType"].(string)}
	if types[0] != "com.apple.security.root" || types[1] != "com.apple.ManagedClient.preferences" {
		t.Fatalf("payloads %v", types)
	}
	if !bytes.Contains(b, []byte("vmbt_secret")) || !bytes.Contains(b, []byte("<string>https://mdm.example.com</string>")) {
		t.Fatal("token or server URL missing")
	}
	m, err := Manifest("https://mdm.example.com/agent/pkg/VibeMDM-Agent.pkg", 123, 10<<20, []string{"abc"}, "1.2", "Acme Agent")
	if err != nil || !bytes.Contains(m, []byte("software-package")) || !bytes.Contains(m, []byte(BundleID)) {
		t.Fatalf("manifest: %s %v", m, err)
	}
}

// fakeXar builds a minimal xar header + zlib TOC, which is all PackageSigner reads.
func fakeXar(t *testing.T, toc string) []byte {
	t.Helper()
	var z bytes.Buffer
	zw := zlib.NewWriter(&z)
	zw.Write([]byte(toc))
	zw.Close()
	var out bytes.Buffer
	out.WriteString("xar!")
	binary.Write(&out, binary.BigEndian, uint16(28))
	binary.Write(&out, binary.BigEndian, uint16(1))
	binary.Write(&out, binary.BigEndian, uint64(z.Len()))
	binary.Write(&out, binary.BigEndian, uint64(len(toc)))
	binary.Write(&out, binary.BigEndian, uint32(1))
	out.Write(z.Bytes())
	return out.Bytes()
}

func TestPackageSigner(t *testing.T) {
	caPEM, keyPEM, _ := NewCA("Acme")
	chain, _ := SignCSR(csrPEM(t, 2048), caPEM, keyPEM)
	blk, _ := pem.Decode([]byte(chain))
	signed := `<xar><toc><signature style="RSA"><KeyInfo><X509Data><X509Certificate>` +
		base64.StdEncoding.EncodeToString(blk.Bytes) + `</X509Certificate></X509Data></KeyInfo></signature></toc></xar>`
	cn, err := PackageSigner(bytes.NewReader(fakeXar(t, signed)))
	if err != nil || cn != "Agent Signing" {
		t.Fatalf("signed package: %q %v", cn, err)
	}
	if _, err := PackageSigner(bytes.NewReader(fakeXar(t, "<xar><toc></toc></xar>"))); err == nil || !strings.Contains(err.Error(), "not signed") {
		t.Fatalf("unsigned package must be refused: %v", err)
	}
	if _, err := PackageSigner(bytes.NewReader([]byte("PK\x03\x04 not a pkg at all......................"))); err == nil {
		t.Fatal("non-xar accepted")
	}
}

func TestChunkMD5s(t *testing.T) {
	sums, err := chunkMD5s(bytes.NewReader(bytes.Repeat([]byte("a"), 25)), 10)
	if err != nil || len(sums) != 3 {
		t.Fatalf("%v %v", sums, err)
	}
}

func TestOldCodeSigningCAIsNotCurrent(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "old"}, NotBefore: time.Now(), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageCodeSigning}}
	der, _ := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if CAIsCurrent(string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))) {
		t.Fatal("a code-signing CA from earlier versions must be replaced")
	}
}
