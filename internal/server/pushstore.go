package server

import (
	"context"
	"crypto/tls"

	"github.com/micromdm/nanomdm/cryptoutil"
	"github.com/micromdm/nanomdm/storage/pgsql"

	"github.com/vibemdm/vibemdm/internal/store"
)

// sealedPushStore wraps NanoMDM's PostgreSQL storage so the APNs private key is
// encrypted at rest. Everything except push cert storage/retrieval passes through.
type sealedPushStore struct {
	*pgsql.PgSQLStorage
	st *store.Store
}

// RetrievePushCert decrypts the stored key before building the TLS keypair.
func (p *sealedPushStore) RetrievePushCert(ctx context.Context, topic string) (*tls.Certificate, string, error) {
	certPEM, keyPEM, stale, err := p.st.RetrievePushCertPEM(ctx, topic)
	if err != nil {
		return nil, "", err
	}
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, "", err
	}
	return &cert, stale, nil
}

// StorePushCert seals the key before storing.
func (p *sealedPushStore) StorePushCert(ctx context.Context, pemCert, pemKey []byte) error {
	topic, err := cryptoutil.TopicFromPEMCert(pemCert)
	if err != nil {
		return err
	}
	return p.st.StorePushCertSealed(ctx, topic, pemCert, pemKey)
}
