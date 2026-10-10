// Package scepsrv implements a minimal SCEP server (RFC 8894) backed by a
// PostgreSQL-stored CA. It issues the device identity certificates used by
// MDM enrollment profiles.
package scepsrv

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/smallstep/scep"
)

// Depot persists the CA and issued certificates. GetSCEPCA must return an
// error matching fs.ErrNotExist when no CA is stored; any other error is fatal.
// CreateSCEPCA must never overwrite an existing CA.
type Depot interface {
	GetSCEPCA(ctx context.Context) (certPEM, keyPEM string, err error)
	SaveSCEPCA(ctx context.Context, certPEM, keyPEM string) error
	CreateSCEPCA(ctx context.Context, certPEM, keyPEM string) error
	NextSCEPSerial(ctx context.Context) (int64, error)
	PutSCEPCert(ctx context.Context, serial string, cn, certPEM string, notBefore, notAfter time.Time) error
}

// ChallengeFunc verifies a SCEP challenge password (and may consume it).
type ChallengeFunc func(ctx context.Context, challenge string) bool

// Server is the SCEP service.
type Server struct {
	depot     Depot
	challenge ChallengeFunc
	log       *slog.Logger
	validity  time.Duration

	mu   sync.RWMutex
	cert *x509.Certificate
	key  *rsa.PrivateKey
	pool *x509.CertPool
}

// New creates a SCEP server, generating a CA if none exists.
func New(ctx context.Context, depot Depot, challenge ChallengeFunc, orgName string, logger *slog.Logger) (*Server, error) {
	s := &Server{depot: depot, challenge: challenge, log: logger, validity: 3 * 365 * 24 * time.Hour}
	if err := s.load(ctx); err != nil {
		if !errors.Is(err, errNoCA) {
			// Never replace a CA we merely failed to read (e.g. wrong
			// encryption key): that would invalidate every enrolled device.
			return nil, fmt.Errorf("loading SCEP CA: %w", err)
		}
		logger.Info("no SCEP CA found, generating a new one")
		certPEM, keyPEM, cert, key, err := generateCA(orgName)
		if err != nil {
			return nil, err
		}
		if err := s.depot.CreateSCEPCA(ctx, certPEM, keyPEM); err != nil {
			return nil, fmt.Errorf("storing new SCEP CA: %w", err)
		}
		s.set(cert, key)
	}
	return s, nil
}

var errNoCA = errors.New("no CA")

func (s *Server) load(ctx context.Context) error {
	certPEM, keyPEM, err := s.depot.GetSCEPCA(ctx)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return errNoCA
		}
		return err
	}
	cert, key, err := ParseCA(certPEM, keyPEM)
	if err != nil {
		return err
	}
	s.set(cert, key)
	return nil
}

func (s *Server) set(cert *x509.Certificate, key *rsa.PrivateKey) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cert, s.key = cert, key
	s.pool = x509.NewCertPool()
	s.pool.AddCert(cert)
}

// ParseCA parses PEM CA material.
func ParseCA(certPEM, keyPEM string) (*x509.Certificate, *rsa.PrivateKey, error) {
	cb, _ := pem.Decode([]byte(certPEM))
	if cb == nil {
		return nil, nil, errors.New("invalid CA certificate PEM")
	}
	cert, err := x509.ParseCertificate(cb.Bytes)
	if err != nil {
		return nil, nil, err
	}
	kb, _ := pem.Decode([]byte(keyPEM))
	if kb == nil {
		return nil, nil, errors.New("invalid CA key PEM")
	}
	var key *rsa.PrivateKey
	if k, err := x509.ParsePKCS1PrivateKey(kb.Bytes); err == nil {
		key = k
	} else if k8, err := x509.ParsePKCS8PrivateKey(kb.Bytes); err == nil {
		rk, ok := k8.(*rsa.PrivateKey)
		if !ok {
			return nil, nil, errors.New("CA key is not RSA")
		}
		key = rk
	} else {
		return nil, nil, errors.New("cannot parse CA key")
	}
	return cert, key, nil
}

// Regenerate replaces the CA with a brand new one. This is an explicit admin
// action: every existing device identity becomes invalid.
func (s *Server) Regenerate(ctx context.Context, orgName string) error {
	certPEM, keyPEM, cert, key, err := generateCA(orgName)
	if err != nil {
		return err
	}
	if err := s.depot.SaveSCEPCA(ctx, certPEM, keyPEM); err != nil {
		return err
	}
	s.set(cert, key)
	return nil
}

// generateCA creates a new self-signed CA keypair.
func generateCA(orgName string) (certPEM, keyPEM string, cert *x509.Certificate, key *rsa.PrivateKey, err error) {
	key, err = rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return
	}
	if orgName == "" {
		orgName = "VibeMDM"
	}
	skid := sha1.Sum(x509.MarshalPKCS1PublicKey(&key.PublicKey))
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 100))
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: orgName + " SCEP CA", Organization: []string{orgName}, OrganizationalUnit: []string{"VibeMDM"}},
		NotBefore:             time.Now().Add(-10 * time.Minute).UTC(),
		NotAfter:              time.Now().AddDate(10, 0, 0).UTC(),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            0,
		SubjectKeyId:          skid[:],
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return
	}
	cert, err = x509.ParseCertificate(der)
	if err != nil {
		return
	}
	certPEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	keyPEM = string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
	return
}

// CACert returns the CA certificate.
func (s *Server) CACert() *x509.Certificate {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cert
}

// CAPEM returns the CA certificate PEM.
func (s *Server) CAPEM() []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: s.CACert().Raw})
}

// Verify checks that a client certificate was issued by our CA (implements NanoMDM's verifier).
func (s *Server) Verify(_ context.Context, cert *x509.Certificate) error {
	if cert == nil {
		return errors.New("missing MDM certificate")
	}
	s.mu.RLock()
	pool := s.pool
	s.mu.RUnlock()
	_, err := cert.Verify(x509.VerifyOptions{Roots: pool, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}})
	return err
}

// signCSR issues a certificate for a decrypted CSR.
func (s *Server) signCSR(ctx context.Context, m *scep.CSRReqMessage) (*x509.Certificate, error) {
	if m == nil || m.CSR == nil {
		return nil, errors.New("missing CSR")
	}
	if err := m.CSR.CheckSignature(); err != nil {
		return nil, fmt.Errorf("CSR signature: %w", err)
	}
	if s.challenge == nil || m.ChallengePassword == "" || !s.challenge(ctx, m.ChallengePassword) {
		return nil, errors.New("invalid SCEP challenge")
	}
	serialN, err := s.depot.NextSCEPSerial(ctx)
	if err != nil {
		return nil, err
	}
	s.mu.RLock()
	caCert, caKey := s.cert, s.key
	s.mu.RUnlock()

	pubDER, err := x509.MarshalPKIXPublicKey(m.CSR.PublicKey)
	if err != nil {
		return nil, err
	}
	skid := sha1.Sum(pubDER)
	now := time.Now().UTC()
	tmpl := &x509.Certificate{
		SerialNumber:   big.NewInt(serialN),
		Subject:        m.CSR.Subject,
		NotBefore:      now.Add(-10 * time.Minute),
		NotAfter:       now.Add(s.validity),
		SubjectKeyId:   skid[:],
		KeyUsage:       x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:    []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		DNSNames:       m.CSR.DNSNames,
		EmailAddresses: m.CSR.EmailAddresses,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, m.CSR.PublicKey, caKey)
	if err != nil {
		return nil, err
	}
	crt, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	certPEM := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	if err := s.depot.PutSCEPCert(ctx, crt.SerialNumber.String(), crt.Subject.CommonName, certPEM, crt.NotBefore, crt.NotAfter); err != nil {
		return nil, err
	}
	return crt, nil
}

// ServeHTTP handles SCEP GET/POST operations.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	op := r.URL.Query().Get("operation")
	switch op {
	case "GetCACaps":
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("Renewal\nSHA-1\nSHA-256\nSHA-512\nAES\nDES3\nSCEPStandard\nPOSTPKIOperation"))
	case "GetCACert":
		w.Header().Set("Content-Type", "application/x-x509-ca-cert")
		w.Write(s.CACert().Raw)
	case "PKIOperation":
		var msg []byte
		var err error
		if r.Method == http.MethodPost {
			msg, err = io.ReadAll(io.LimitReader(r.Body, 1<<20))
		} else {
			raw := r.URL.Query().Get("message")
			// some clients send URL-encoded base64 with spaces for '+'
			raw = strings.ReplaceAll(raw, " ", "+")
			msg, err = base64.StdEncoding.DecodeString(raw)
		}
		if err != nil || len(msg) == 0 {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		resp, err := s.pkiOperation(r.Context(), msg)
		if err != nil {
			s.log.Warn("scep pkioperation failed", "err", err, "remote", r.RemoteAddr)
			http.Error(w, "scep error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/x-pki-message")
		w.Write(resp)
	case "GetNextCACert":
		http.Error(w, "not implemented", http.StatusNotImplemented)
	default:
		http.Error(w, "unknown operation", http.StatusBadRequest)
	}
}

func (s *Server) pkiOperation(ctx context.Context, data []byte) ([]byte, error) {
	s.mu.RLock()
	caCert, caKey := s.cert, s.key
	s.mu.RUnlock()
	// Note: do not pass WithCACerts here; the client's PKCSReq is signed with
	// its own (self-signed) certificate which is embedded in the message.
	msg, err := scep.ParsePKIMessage(data)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	if err := msg.DecryptPKIEnvelope(caCert, caKey); err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}
	crt, err := s.signCSR(ctx, msg.CSRReqMessage)
	if err != nil {
		s.log.Warn("scep: refusing to sign CSR", "err", err)
		rep, ferr := msg.Fail(caCert, caKey, scep.BadRequest)
		if ferr != nil {
			return nil, ferr
		}
		return rep.Raw, nil
	}
	s.log.Info("scep: issued identity certificate", "cn", crt.Subject.CommonName, "serial", crt.SerialNumber.String())
	rep, err := msg.Success(caCert, caKey, crt)
	if err != nil {
		return nil, err
	}
	return rep.Raw, nil
}
