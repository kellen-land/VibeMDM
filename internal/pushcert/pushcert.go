// Package pushcert implements the APNs MDM push certificate workflow:
// generating a CSR, requesting vendor signing via mdmcert.download,
// decrypting the response, and storing the final certificate.
package pushcert

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/smallstep/pkcs7"
)

// mdmcert.download is a free community service (run by the MicroMDM
// maintainers) that signs MDM push CSRs with a vendor certificate so
// self-hosters can obtain an APNs push certificate from Apple. This is the
// same public API key the MicroMDM tooling uses.
const (
	MDMCertDownloadURL    = "https://mdmcert.download/api/v1/signrequest"
	mdmcertDownloadAPIKey = "f847aea2ba06b41264d587b229e2712c89b1490a1208b7ff1aafab5bb40d47bc"
)

// Request holds the generated key material.
type Request struct {
	KeyPEM  string
	CSRPEM  string
	CertPEM string // self-signed encryption cert the response is encrypted to
}

// NewRequest generates a key, CSR and encryption certificate.
func NewRequest(orgName, email string) (*Request, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	if orgName == "" {
		orgName = "VibeMDM"
	}
	csrTmpl := &x509.CertificateRequest{
		Subject:            pkix.Name{CommonName: orgName + " MDM Push", Organization: []string{orgName}, Country: []string{"US"}},
		SignatureAlgorithm: x509.SHA256WithRSA,
	}
	if email != "" {
		csrTmpl.EmailAddresses = []string{email}
	}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, csrTmpl, key)
	if err != nil {
		return nil, err
	}
	skid := sha1.Sum(x509.MarshalPKCS1PublicKey(&key.PublicKey))
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 100))
	certTmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: orgName + " MDM Push Request Encryption", Organization: []string{orgName}},
		NotBefore:    time.Now().Add(-10 * time.Minute),
		NotAfter:     time.Now().AddDate(1, 0, 0),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		SubjectKeyId: skid[:],
	}
	certDER, err := x509.CreateCertificate(rand.Reader, certTmpl, certTmpl, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	return &Request{
		KeyPEM:  string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})),
		CSRPEM:  string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER})),
		CertPEM: string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})),
	}, nil
}

// SubmitToMDMCertDownload asks mdmcert.download to sign the CSR. The signed
// (encrypted) request is emailed to the address.
func SubmitToMDMCertDownload(ctx context.Context, client *http.Client, req *Request, email string) error {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	if email == "" {
		return errors.New("email address required")
	}
	body, _ := json.Marshal(map[string]string{
		"csr":     base64.StdEncoding.EncodeToString([]byte(req.CSRPEM)),
		"email":   email,
		"key":     mdmcertDownloadAPIKey,
		"encrypt": base64.StdEncoding.EncodeToString([]byte(req.CertPEM)),
	})
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, MDMCertDownloadURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("User-Agent", "vibemdm")
	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("contacting mdmcert.download: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("mdmcert.download returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	var out struct {
		Result string `json:"result"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(respBody, &out); err == nil && out.Result != "" && out.Result != "success" {
		return fmt.Errorf("mdmcert.download: %s %s", out.Result, out.Reason)
	}
	return nil
}

// DecryptSignedRequest decrypts the .p7 file emailed by mdmcert.download and
// returns the vendor-signed push certificate request (a plist) to upload to
// https://identity.apple.com/pushcert/.
func DecryptSignedRequest(encrypted []byte, req *Request) ([]byte, error) {
	der := decodeFlexible(encrypted)
	p7, err := pkcs7.Parse(der)
	if err != nil {
		return nil, fmt.Errorf("parsing encrypted request: %w", err)
	}
	cb, _ := pem.Decode([]byte(req.CertPEM))
	kb, _ := pem.Decode([]byte(req.KeyPEM))
	if cb == nil || kb == nil {
		return nil, errors.New("missing request key material")
	}
	cert, err := x509.ParseCertificate(cb.Bytes)
	if err != nil {
		return nil, err
	}
	key, err := x509.ParsePKCS1PrivateKey(kb.Bytes)
	if err != nil {
		return nil, err
	}
	content, err := p7.Decrypt(cert, key)
	if err != nil {
		return nil, fmt.Errorf("decrypting: %w (was this file generated for the current request?)", err)
	}
	return content, nil
}

func decodeFlexible(in []byte) []byte {
	// Raw DER (a SEQUENCE) is used as is: trimming would eat 0x09/0x0A/0x20 bytes that are
	// part of the binary data at either end.
	if len(in) > 0 && in[0] == 0x30 {
		if _, err := pkcs7.Parse(in); err == nil {
			return in
		}
	}
	trim := bytes.TrimSpace(in)
	if b, err := hex.DecodeString(string(trim)); err == nil && len(b) > 0 {
		return b
	}
	if b, err := base64.StdEncoding.DecodeString(string(trim)); err == nil && len(b) > 0 {
		return b
	}
	if blk, _ := pem.Decode(trim); blk != nil {
		return blk.Bytes
	}
	return trim
}

// NormalizeCertificatePEM accepts PEM or DER and returns PEM.
func NormalizeCertificatePEM(in []byte) (string, *x509.Certificate, error) {
	trim := bytes.TrimSpace(in)
	var der []byte
	if blk, _ := pem.Decode(trim); blk != nil {
		der = blk.Bytes
	} else {
		der = trim
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return "", nil, fmt.Errorf("parsing certificate: %w", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})), cert, nil
}

// NormalizeKeyPEM accepts PKCS#1 or PKCS#8 RSA private keys and returns PKCS#1 PEM.
func NormalizeKeyPEM(in []byte) (string, *rsa.PrivateKey, error) {
	blk, _ := pem.Decode(bytes.TrimSpace(in))
	if blk == nil {
		return "", nil, errors.New("private key must be PEM encoded")
	}
	if x509.IsEncryptedPEMBlock(blk) { //nolint:staticcheck
		return "", nil, errors.New("encrypted private keys are not supported; decrypt it first")
	}
	var key *rsa.PrivateKey
	if k, err := x509.ParsePKCS1PrivateKey(blk.Bytes); err == nil {
		key = k
	} else if k8, err := x509.ParsePKCS8PrivateKey(blk.Bytes); err == nil {
		rk, ok := k8.(*rsa.PrivateKey)
		if !ok {
			return "", nil, errors.New("push private key must be RSA")
		}
		key = rk
	} else {
		return "", nil, errors.New("cannot parse private key")
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})), key, nil
}

// KeyMatchesCert verifies the key pair.
func KeyMatchesCert(cert *x509.Certificate, key *rsa.PrivateKey) bool {
	pub, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok {
		return false
	}
	return pub.N.Cmp(key.N) == 0 && pub.E == key.E
}

// TopicFromCert extracts the APNs topic (UID) from a push certificate.
func TopicFromCert(cert *x509.Certificate) (string, error) {
	for _, n := range cert.Subject.Names {
		// id-userid (UID) OID 0.9.2342.19200300.100.1.1
		if n.Type.String() == "0.9.2342.19200300.100.1.1" {
			if s, ok := n.Value.(string); ok {
				return s, nil
			}
		}
	}
	return "", errors.New("certificate has no UID (topic); is this an Apple MDM push certificate?")
}
