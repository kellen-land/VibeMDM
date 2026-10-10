// Package enroll generates the MDM enrollment profile (.mobileconfig).
package enroll

import (
	"bytes"
	"crypto"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"

	"github.com/smallstep/pkcs7"
	"howett.net/plist"
)

// Params configures the enrollment profile.
type Params struct {
	ServerURL     string // https://mdm.example.com
	Topic         string // APNs topic from the push certificate
	SCEPChallenge string
	OrgName       string
	DisplayName   string
	Identifier    string // e.g. com.vibemdm.enrollment
	Description   string
	// Optional PKCS#7 signing identity.
	SigningCertPEM string
	SigningKeyPEM  string
}

// Stable UUIDs derived so the same server always produces the same profile identity.
const (
	scepPayloadUUID = "4B1A8E2C-7C33-4F2B-9A1D-5E6F7A8B9C0D"
	mdmPayloadUUID  = "9D2B6F4A-1E3C-4D5B-8A7F-0C1D2E3F4A5B"
	profileUUID     = "1F2E3D4C-5B6A-4798-8A9B-0C1D2E3F4A5B"
)

type scepContent struct {
	URL        string       `plist:"URL"`
	Name       string       `plist:"Name"`
	Subject    [][][]string `plist:"Subject"`
	Challenge  string       `plist:"Challenge,omitempty"`
	Keysize    int          `plist:"Keysize"`
	KeyType    string       `plist:"Key Type"`
	KeyUsage   int          `plist:"Key Usage"`
	Retries    int          `plist:"Retries"`
	RetryDelay int          `plist:"RetryDelay"`
}

type scepPayload struct {
	PayloadContent      scepContent `plist:"PayloadContent"`
	PayloadDescription  string      `plist:"PayloadDescription"`
	PayloadDisplayName  string      `plist:"PayloadDisplayName"`
	PayloadIdentifier   string      `plist:"PayloadIdentifier"`
	PayloadOrganization string      `plist:"PayloadOrganization"`
	PayloadType         string      `plist:"PayloadType"`
	PayloadUUID         string      `plist:"PayloadUUID"`
	PayloadVersion      int         `plist:"PayloadVersion"`
}

type mdmPayload struct {
	AccessRights            int      `plist:"AccessRights"`
	CheckInURL              string   `plist:"CheckInURL"`
	CheckOutWhenRemoved     bool     `plist:"CheckOutWhenRemoved"`
	IdentityCertificateUUID string   `plist:"IdentityCertificateUUID"`
	PayloadDescription      string   `plist:"PayloadDescription"`
	PayloadDisplayName      string   `plist:"PayloadDisplayName"`
	PayloadIdentifier       string   `plist:"PayloadIdentifier"`
	PayloadOrganization     string   `plist:"PayloadOrganization"`
	PayloadType             string   `plist:"PayloadType"`
	PayloadUUID             string   `plist:"PayloadUUID"`
	PayloadVersion          int      `plist:"PayloadVersion"`
	ServerCapabilities      []string `plist:"ServerCapabilities"`
	ServerURL               string   `plist:"ServerURL"`
	SignMessage             bool     `plist:"SignMessage"`
	Topic                   string   `plist:"Topic"`
}

type profile struct {
	PayloadContent           []any  `plist:"PayloadContent"`
	PayloadDescription       string `plist:"PayloadDescription"`
	PayloadDisplayName       string `plist:"PayloadDisplayName"`
	PayloadIdentifier        string `plist:"PayloadIdentifier"`
	PayloadOrganization      string `plist:"PayloadOrganization"`
	PayloadRemovalDisallowed bool   `plist:"PayloadRemovalDisallowed"`
	PayloadScope             string `plist:"PayloadScope"`
	PayloadType              string `plist:"PayloadType"`
	PayloadUUID              string `plist:"PayloadUUID"`
	PayloadVersion           int    `plist:"PayloadVersion"`
}

// Build renders the enrollment profile XML, signing it if a signing identity is configured.
func Build(p Params) ([]byte, error) {
	if p.ServerURL == "" {
		return nil, errors.New("server URL is not configured")
	}
	if p.Topic == "" {
		return nil, errors.New("no APNs push certificate uploaded (topic unknown)")
	}
	if p.SCEPChallenge == "" {
		return nil, errors.New("SCEP challenge not configured")
	}
	base := strings.TrimRight(p.ServerURL, "/")
	if p.Identifier == "" {
		p.Identifier = "com.vibemdm.enrollment"
	}
	if p.DisplayName == "" {
		p.DisplayName = "VibeMDM Enrollment"
	}
	if p.OrgName == "" {
		p.OrgName = "VibeMDM"
	}
	if p.Description == "" {
		p.Description = "Enrolls this Mac in " + p.OrgName + " device management."
	}
	prof := profile{
		PayloadContent: []any{
			scepPayload{
				PayloadContent: scepContent{
					URL:        base + "/scep",
					Name:       p.OrgName + " SCEP",
					Subject:    [][][]string{{{"O", p.OrgName}}, {{"CN", p.OrgName + " Device Identity"}}},
					Challenge:  p.SCEPChallenge,
					Keysize:    2048,
					KeyType:    "RSA",
					KeyUsage:   5,
					Retries:    3,
					RetryDelay: 10,
				},
				PayloadDescription:  "Configures the device identity certificate.",
				PayloadDisplayName:  "Device Identity (SCEP)",
				PayloadIdentifier:   p.Identifier + ".scep",
				PayloadOrganization: p.OrgName,
				PayloadType:         "com.apple.security.scep",
				PayloadUUID:         scepPayloadUUID,
				PayloadVersion:      1,
			},
			mdmPayload{
				AccessRights:            8191,
				CheckInURL:              base + "/mdm",
				CheckOutWhenRemoved:     true,
				IdentityCertificateUUID: scepPayloadUUID,
				PayloadDescription:      "Configures Mobile Device Management.",
				PayloadDisplayName:      "Mobile Device Management",
				PayloadIdentifier:       p.Identifier + ".mdm",
				PayloadOrganization:     p.OrgName,
				PayloadType:             "com.apple.mdm",
				PayloadUUID:             mdmPayloadUUID,
				PayloadVersion:          1,
				ServerCapabilities:      []string{"com.apple.mdm.per-user-connections", "com.apple.mdm.bootstraptoken", "com.apple.mdm.token"},
				ServerURL:               base + "/mdm",
				SignMessage:             true,
				Topic:                   p.Topic,
			},
		},
		PayloadDescription:       p.Description,
		PayloadDisplayName:       p.DisplayName,
		PayloadIdentifier:        p.Identifier,
		PayloadOrganization:      p.OrgName,
		PayloadRemovalDisallowed: false,
		PayloadScope:             "System",
		PayloadType:              "Configuration",
		PayloadUUID:              profileUUID,
		PayloadVersion:           1,
	}
	var buf bytes.Buffer
	enc := plist.NewEncoder(&buf)
	enc.Indent("\t")
	if err := enc.Encode(prof); err != nil {
		return nil, err
	}
	out := buf.Bytes()
	if p.SigningCertPEM != "" && p.SigningKeyPEM != "" {
		signed, err := Sign(out, p.SigningCertPEM, p.SigningKeyPEM)
		if err != nil {
			return nil, fmt.Errorf("signing profile: %w", err)
		}
		return signed, nil
	}
	return out, nil
}

// Sign wraps a profile in a PKCS#7 SignedData structure using the given identity.
// The cert PEM may contain intermediate certificates after the leaf.
func Sign(data []byte, certPEM, keyPEM string) ([]byte, error) {
	var certs []*x509.Certificate
	rest := []byte(certPEM)
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		c, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, err
		}
		certs = append(certs, c)
	}
	if len(certs) == 0 {
		return nil, errors.New("no certificate in signing PEM")
	}
	kb, _ := pem.Decode([]byte(keyPEM))
	if kb == nil {
		return nil, errors.New("invalid signing key PEM")
	}
	var key crypto.PrivateKey
	if k, err := x509.ParsePKCS8PrivateKey(kb.Bytes); err == nil {
		key = k
	} else if k, err := x509.ParsePKCS1PrivateKey(kb.Bytes); err == nil {
		key = k
	} else if k, err := x509.ParseECPrivateKey(kb.Bytes); err == nil {
		key = k
	} else {
		return nil, errors.New("cannot parse signing key")
	}
	sd, err := pkcs7.NewSignedData(data)
	if err != nil {
		return nil, err
	}
	sd.SetDigestAlgorithm(pkcs7.OIDDigestAlgorithmSHA256)
	signer, ok := key.(crypto.Signer)
	if !ok {
		return nil, errors.New("signing key is not a crypto.Signer")
	}
	if err := sd.AddSignerChain(certs[0], signer, certs[1:], pkcs7.SignerInfoConfig{}); err != nil {
		return nil, err
	}
	return sd.Finish()
}
