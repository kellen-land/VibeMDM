package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto"
	"fmt"
	"os"

	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/json"
	"errors"
	"github.com/smallstep/pkcs7"

	"github.com/vibemdm/vibemdm/internal/model"
)

func jsonRoundTrip(v any) ([]byte, error) { return json.Marshal(v) }

func decodePolicy(v any) (model.AgentPolicy, bool) {
	var ap model.AgentPolicy
	b, err := json.Marshal(v)
	if err != nil {
		return ap, false
	}
	if err := json.Unmarshal(b, &ap); err != nil || ap.PolicyID == "" {
		return ap, false
	}
	return ap, true
}

// ---- CSR with challengePassword ----
//
// Go's x509 package cannot emit the PKCS#9 challengePassword attribute, so we
// re-assemble the CertificationRequestInfo with the attribute and re-sign it.

var oidChallengePassword = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 7}

type certificationRequest struct {
	Raw                      asn1.RawContent
	CertificationRequestInfo certificationRequestInfo
	SignatureAlgorithm       asn1.RawValue
	SignatureValue           asn1.BitString
}

type certificationRequestInfo struct {
	Raw           asn1.RawContent
	Version       int
	Subject       asn1.RawValue
	PublicKey     asn1.RawValue
	RawAttributes []asn1.RawValue `asn1:"tag:0"`
}

type pkcs9Attribute struct {
	Type   asn1.ObjectIdentifier
	Values []asn1.RawValue `asn1:"set"`
}

func addChallengePassword(csrDER []byte, key *rsa.PrivateKey, challenge string) ([]byte, error) {
	var req certificationRequest
	rest, err := asn1.Unmarshal(csrDER, &req)
	if err != nil {
		return nil, err
	}
	if len(rest) > 0 {
		return nil, errors.New("trailing data in CSR")
	}
	val, err := asn1.MarshalWithParams(challenge, "utf8")
	if err != nil {
		return nil, err
	}
	attr := pkcs9Attribute{Type: oidChallengePassword, Values: []asn1.RawValue{{FullBytes: val}}}
	attrDER, err := asn1.Marshal(attr)
	if err != nil {
		return nil, err
	}
	info := req.CertificationRequestInfo
	info.RawAttributes = append(info.RawAttributes, asn1.RawValue{FullBytes: attrDER})
	info.Raw = nil
	infoDER, err := asn1.Marshal(info)
	if err != nil {
		return nil, err
	}
	// Re-sign: sha256WithRSAEncryption
	h := sha256.Sum256(infoDER)
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, h[:])
	if err != nil {
		return nil, err
	}
	sigAlg, _ := asn1.Marshal(struct {
		Algorithm  asn1.ObjectIdentifier
		Parameters asn1.RawValue `asn1:"optional"`
	}{Algorithm: asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 11}, Parameters: asn1.NullRawValue})
	out := struct {
		Info      asn1.RawValue
		SigAlg    asn1.RawValue
		Signature asn1.BitString
	}{Info: asn1.RawValue{FullBytes: infoDER}, SigAlg: asn1.RawValue{FullBytes: sigAlg}, Signature: asn1.BitString{Bytes: sig, BitLength: len(sig) * 8}}
	der, err := asn1.Marshal(out)
	if err != nil {
		return nil, err
	}
	// sanity check
	if _, err := x509.ParseCertificateRequest(der); err != nil {
		return nil, err
	}
	return der, nil
}

// escrowedKeyCMS encrypts this device's recovery key to the certificate in an
// installed FDERecoveryKeyEscrow profile, or returns nil if none is installed.
func (d *device) escrowedKeyCMS() []byte {
	for _, prof := range d.profiles {
		payloads, _ := prof["PayloadContent"].([]any)
		var cert *x509.Certificate
		hasEscrow := false
		for _, it := range payloads {
			m, _ := it.(map[string]any)
			switch m["PayloadType"] {
			case "com.apple.security.FDERecoveryKeyEscrow":
				hasEscrow = true
			case "com.apple.security.pkcs1":
				if der, ok := m["PayloadContent"].([]byte); ok {
					cert, _ = x509.ParseCertificate(der)
				}
			}
		}
		if hasEscrow && cert != nil {
			prk := fmt.Sprintf("SIM%d-%04d-%04d-%04d-%04d-%04d", d.idx, d.prkGen, d.idx*1111%10000, d.idx*7%10000, d.prkGen*13%10000, d.idx*97%10000)
			cms, err := pkcs7.Encrypt([]byte(prk), []*x509.Certificate{cert})
			if err != nil {
				return nil
			}
			return cms
		}
	}
	return nil
}

// writeFakeBundle writes a tiny diagnostics tarball for the simulator.
func writeFakeBundle(path, name string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	body := []byte("simulated diagnostics for " + name + "\n")
	tw.WriteHeader(&tar.Header{Name: "vibemdm-diagnostics/README.txt", Mode: 0o600, Size: int64(len(body))})
	tw.Write(body)
	tw.Close()
	return gz.Close()
}
