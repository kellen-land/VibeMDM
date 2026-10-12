// Package agentinstall installs the VibeMDM agent on a Mac through MDM right after it
// enrolls, the way Jamf Pro installs its management framework after user-initiated
// enrollment:
//
//  1. a temporary "bootstrap" profile gives the Mac the package-signing root (so macOS
//     trusts the package) and a single-use agent enrollment token (managed preferences
//     in the com.vibemdm.agent domain);
//  2. InstallEnterpriseApplication installs the signed agent package from a manifest;
//  3. the package's postinstall runs `vibemdm-agent enroll --from-mdm`, which reads the
//     token from managed preferences and enrolls;
//  4. when the agent enrolls, the bootstrap profile is removed again, so the root is
//     trusted only for the minutes the install takes.
//
// The package is built and signed on a Mac with build-agent-pkg.sh (by hand, or from the
// console with Build on a Mac). The signing key never leaves that Mac: the script sends a
// CSR and this package's CA signs an installer-signing certificate. productsign only
// accepts it when the CA is trusted on the build Mac (a configuration profile) and the
// certificate carries Apple's installer-signing usage and no other.
package agentinstall

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"errors"
	"math/big"
	"time"
)

// newSerial returns a random 128-bit certificate serial number.
func newSerial() (*big.Int, error) {
	return rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
}

// OIDInstallerSigning is Apple's extended key usage for signing installer packages (the one in
// Developer ID Installer certificates). productsign refuses a certificate without it, and also
// one that carries the generic code-signing usage next to it (verified on macOS 26.6.2).
var OIDInstallerSigning = asn1.ObjectIdentifier{1, 2, 840, 113635, 100, 4, 13}

// NewCA creates the package-signing CA. It may only issue installer-signing certificates
// (its extended key usage, path length 0, and a DNS name constraint that permits only
// "invalid." so the root cannot vouch for a TLS server even while a Mac trusts it).
func NewCA(orgName string) (certPEM, keyPEM string, err error) {
	key, err := rsa.GenerateKey(rand.Reader, 3072)
	if err != nil {
		return "", "", err
	}
	serial, err := newSerial()
	if err != nil {
		return "", "", err
	}
	if orgName == "" {
		orgName = "VibeMDM"
	}
	tmpl := &x509.Certificate{
		SerialNumber:                serial,
		Subject:                     pkix.Name{Organization: []string{orgName}, CommonName: orgName + " Package Signing CA"},
		NotBefore:                   time.Now().Add(-time.Hour),
		NotAfter:                    time.Now().AddDate(10, 0, 0),
		KeyUsage:                    x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		UnknownExtKeyUsage:          []asn1.ObjectIdentifier{OIDInstallerSigning},
		BasicConstraintsValid:       true,
		IsCA:                        true,
		MaxPathLen:                  0,
		MaxPathLenZero:              true,
		PermittedDNSDomainsCritical: true,
		PermittedDNSDomains:         []string{"invalid"},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return "", "", err
	}
	certPEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	keyPEM = string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
	return certPEM, keyPEM, nil
}

// LeafValidity is how long a package-signing certificate is valid. Packages stay
// installable after it expires only while macOS accepts the signature's timestamp,
// so build a new package (which issues a new certificate) before then.
const LeafValidity = 2 * 365 * 24 * time.Hour

// SignCSR issues an installer-signing certificate for a CSR made on the build Mac and returns
// the leaf followed by the CA certificate (PEM).
func SignCSR(csrPEM []byte, caCertPEM, caKeyPEM string) (string, error) {
	blk, _ := pem.Decode(csrPEM)
	if blk == nil || blk.Type != "CERTIFICATE REQUEST" {
		return "", errors.New("expected a PEM certificate request")
	}
	csr, err := x509.ParseCertificateRequest(blk.Bytes)
	if err != nil {
		return "", err
	}
	if err := csr.CheckSignature(); err != nil {
		return "", err
	}
	if k, ok := csr.PublicKey.(*rsa.PublicKey); !ok || k.N.BitLen() < 2048 {
		return "", errors.New("the request must use an RSA key of at least 2048 bits")
	}
	caCert, caKey, err := parseCA(caCertPEM, caKeyPEM)
	if err != nil {
		return "", err
	}
	serial, err := newSerial()
	if err != nil {
		return "", err
	}
	cn := csr.Subject.CommonName
	if cn == "" || len(cn) > 64 {
		cn = "VibeMDM Agent Package Signing"
	}
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{Organization: caCert.Subject.Organization, CommonName: cn},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(LeafValidity),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		UnknownExtKeyUsage:    []asn1.ObjectIdentifier{OIDInstallerSigning},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, csr.PublicKey, caKey)
	if err != nil {
		return "", err
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})) + caCertPEM, nil
}

func parseCA(certPEM, keyPEM string) (*x509.Certificate, *rsa.PrivateKey, error) {
	cb, _ := pem.Decode([]byte(certPEM))
	kb, _ := pem.Decode([]byte(keyPEM))
	if cb == nil || kb == nil {
		return nil, nil, errors.New("package-signing CA is unreadable")
	}
	cert, err := x509.ParseCertificate(cb.Bytes)
	if err != nil {
		return nil, nil, err
	}
	key, err := x509.ParsePKCS1PrivateKey(kb.Bytes)
	if err != nil {
		return nil, nil, err
	}
	return cert, key, nil
}

// CADER returns the DER bytes of a PEM certificate.
func CADER(certPEM string) ([]byte, error) {
	b, _ := pem.Decode([]byte(certPEM))
	if b == nil {
		return nil, errors.New("not a PEM certificate")
	}
	return b.Bytes, nil
}

// Fingerprint returns the SHA-256 fingerprint of a PEM certificate (hex, upper case pairs).
func Fingerprint(certPEM string) string {
	der, err := CADER(certPEM)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(der)
	const hexd = "0123456789ABCDEF"
	out := make([]byte, 0, len(sum)*3)
	for i, b := range sum {
		if i > 0 {
			out = append(out, ':')
		}
		out = append(out, hexd[b>>4], hexd[b&15])
	}
	return string(out)
}

// CAIsCurrent reports whether a stored CA can issue certificates productsign accepts (CAs made
// by earlier versions carried the code-signing usage, which productsign refuses).
func CAIsCurrent(certPEM string) bool {
	b, _ := pem.Decode([]byte(certPEM))
	if b == nil {
		return false
	}
	c, err := x509.ParseCertificate(b.Bytes)
	if err != nil {
		return false
	}
	for _, o := range c.UnknownExtKeyUsage {
		if o.Equal(OIDInstallerSigning) {
			return len(c.ExtKeyUsage) == 0
		}
	}
	return false
}
