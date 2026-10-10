// Package escrow implements FileVault personal recovery key (PRK) escrow the
// way Apple's MDM protocol provides it: an FDERecoveryKeyEscrow configuration
// profile carrying the server's encryption certificate makes macOS hand the
// PRK to the MDM, CMS-encrypted, in the SecurityInfo response
// (FDE_PersonalRecoveryKeyCMS). The server decrypts it with its private key.
package escrow

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/smallstep/pkcs7"
	"howett.net/plist"
)

// Identity is the escrow encryption certificate and key.
type Identity struct {
	Cert *x509.Certificate
	Key  *rsa.PrivateKey
}

// NewIdentity generates a 10-year self-signed encryption certificate.
func NewIdentity(orgName string) (*Identity, string, string, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, "", "", err
	}
	if orgName == "" {
		orgName = "VibeMDM"
	}
	skid := sha1.Sum(x509.MarshalPKCS1PublicKey(&key.PublicKey))
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 100))
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: orgName + " FileVault Escrow", Organization: []string{orgName}},
		NotBefore:    time.Now().Add(-10 * time.Minute).UTC(),
		NotAfter:     time.Now().AddDate(10, 0, 0).UTC(),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDataEncipherment | x509.KeyUsageDigitalSignature,
		SubjectKeyId: skid[:],
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, "", "", err
	}
	cert, _ := x509.ParseCertificate(der)
	certPEM := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	keyPEM := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
	return &Identity{Cert: cert, Key: key}, certPEM, keyPEM, nil
}

// ParseIdentity loads PEM material.
func ParseIdentity(certPEM, keyPEM string) (*Identity, error) {
	cb, _ := pem.Decode([]byte(certPEM))
	kb, _ := pem.Decode([]byte(keyPEM))
	if cb == nil || kb == nil {
		return nil, errors.New("invalid escrow identity PEM")
	}
	cert, err := x509.ParseCertificate(cb.Bytes)
	if err != nil {
		return nil, err
	}
	key, err := x509.ParsePKCS1PrivateKey(kb.Bytes)
	if err != nil {
		if k8, e8 := x509.ParsePKCS8PrivateKey(kb.Bytes); e8 == nil {
			if rk, ok := k8.(*rsa.PrivateKey); ok {
				return &Identity{Cert: cert, Key: rk}, nil
			}
		}
		return nil, err
	}
	return &Identity{Cert: cert, Key: key}, nil
}

// DecryptPRK decrypts the CMS blob macOS returns in SecurityInfo.
func (id *Identity) DecryptPRK(cms []byte) (string, error) {
	if len(cms) == 0 {
		return "", errors.New("empty CMS")
	}
	p7, err := pkcs7.Parse(cms)
	if err != nil {
		return "", fmt.Errorf("parsing PRK CMS: %w", err)
	}
	pt, err := p7.Decrypt(id.Cert, id.Key)
	if err != nil {
		return "", fmt.Errorf("decrypting PRK: %w", err)
	}
	key := strings.TrimSpace(string(pt))
	if key == "" {
		return "", errors.New("decrypted PRK is empty")
	}
	return key, nil
}

// Profile identifiers for the built-in FileVault profiles.
const (
	EscrowProfileIdentifier  = "com.vibemdm.filevault.escrow"
	EnforceProfileIdentifier = "com.vibemdm.filevault.enforce"
	escrowProfileUUID        = "6E3C1F2A-7B4D-4C9E-9A1B-2C3D4E5F6A7B"
	escrowCertPayloadUUID    = "6E3C1F2A-7B4D-4C9E-9A1B-2C3D4E5F6A7C"
	escrowPayloadUUID        = "6E3C1F2A-7B4D-4C9E-9A1B-2C3D4E5F6A7D"
	enforceProfileUUID       = "6E3C1F2A-7B4D-4C9E-9A1B-2C3D4E5F6A8B"
	enforcePayloadUUID       = "6E3C1F2A-7B4D-4C9E-9A1B-2C3D4E5F6A8C"
)

func encode(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := plist.NewEncoder(&buf)
	enc.Indent("\t")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// EscrowProfile builds the FDERecoveryKeyEscrow configuration profile.
func (id *Identity) EscrowProfile(orgName string) ([]byte, error) {
	if orgName == "" {
		orgName = "VibeMDM"
	}
	return encode(map[string]any{
		"PayloadContent": []any{
			map[string]any{
				"PayloadType": "com.apple.security.pkcs1", "PayloadVersion": 1, "PayloadIdentifier": EscrowProfileIdentifier + ".cert",
				"PayloadUUID": escrowCertPayloadUUID, "PayloadDisplayName": "FileVault Escrow Certificate", "PayloadOrganization": orgName,
				"PayloadContent": id.Cert.Raw, "PayloadCertificateFileName": "escrow.cer",
			},
			map[string]any{
				"PayloadType": "com.apple.security.FDERecoveryKeyEscrow", "PayloadVersion": 1, "PayloadIdentifier": EscrowProfileIdentifier + ".fde",
				"PayloadUUID": escrowPayloadUUID, "PayloadDisplayName": "FileVault Recovery Key Escrow", "PayloadOrganization": orgName,
				"Location":               orgName,
				"EncryptCertPayloadUUID": escrowCertPayloadUUID,
				"DeviceKey":              "",
			},
		},
		"PayloadDescription": "Escrows the FileVault personal recovery key to " + orgName + " device management.",
		"PayloadDisplayName": orgName + " FileVault Recovery Key Escrow", "PayloadIdentifier": EscrowProfileIdentifier,
		"PayloadOrganization": orgName, "PayloadScope": "System", "PayloadType": "Configuration", "PayloadUUID": escrowProfileUUID, "PayloadVersion": 1,
	})
}

// EnforceOptions configure the FileVault enforcement profile.
type EnforceOptions struct {
	DeferAtLogout        bool // prompt at logout instead of immediately
	DeferDontAskAtLogout bool
	MaxBypassAttempts    int // -1 = unlimited deferrals, 0..9999
	ShowRecoveryKey      bool
}

// EnforceProfile builds a com.apple.MCX.FileVault2 profile that turns FileVault
// on with a personal recovery key (which the escrow profile then captures).
func EnforceProfile(orgName string, o EnforceOptions) ([]byte, error) {
	if orgName == "" {
		orgName = "VibeMDM"
	}
	fv := map[string]any{
		"PayloadType": "com.apple.MCX.FileVault2", "PayloadVersion": 1, "PayloadIdentifier": EnforceProfileIdentifier + ".fv2",
		"PayloadUUID": enforcePayloadUUID, "PayloadDisplayName": "FileVault 2", "PayloadOrganization": orgName,
		"Enable": "On", "Defer": true, "UseRecoveryKey": true, "ShowRecoveryKey": o.ShowRecoveryKey,
		"DeferForceAtUserLoginMaxBypassAttempts": o.MaxBypassAttempts,
		"DeferDontAskAtUserLogout":               o.DeferDontAskAtLogout,
	}
	if !o.DeferAtLogout {
		fv["DeferDontAskAtUserLogout"] = true
	}
	return encode(map[string]any{
		"PayloadContent":     []any{fv},
		"PayloadDescription": "Requires FileVault disk encryption with a personal recovery key.",
		"PayloadDisplayName": orgName + " FileVault Enforcement", "PayloadIdentifier": EnforceProfileIdentifier,
		"PayloadOrganization": orgName, "PayloadScope": "System", "PayloadType": "Configuration", "PayloadUUID": enforceProfileUUID, "PayloadVersion": 1,
	})
}

// Fingerprint returns the display fingerprint of a PRK (last block).
func Fingerprint(prk string) string {
	parts := strings.Split(prk, "-")
	last := parts[len(parts)-1]
	if len(last) > 4 {
		last = last[len(last)-4:]
	}
	return "…-" + last
}

// ParseEnforceOptions reads the options back from an enforcement profile.
func ParseEnforceOptions(profile []byte) (*EnforceOptions, error) {
	var doc struct {
		PayloadContent []map[string]any
	}
	if _, err := plist.Unmarshal(profile, &doc); err != nil {
		return nil, err
	}
	for _, p := range doc.PayloadContent {
		if p["PayloadType"] != "com.apple.MCX.FileVault2" {
			continue
		}
		o := &EnforceOptions{}
		dontAsk, _ := p["DeferDontAskAtUserLogout"].(bool)
		o.DeferDontAskAtLogout, o.DeferAtLogout = dontAsk, !dontAsk
		o.ShowRecoveryKey, _ = p["ShowRecoveryKey"].(bool)
		switch n := p["DeferForceAtUserLoginMaxBypassAttempts"].(type) {
		case int64:
			o.MaxBypassAttempts = int(n)
		case uint64:
			o.MaxBypassAttempts = int(n)
		}
		return o, nil
	}
	return nil, errors.New("no FileVault2 payload")
}
