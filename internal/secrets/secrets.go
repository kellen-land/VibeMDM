// Package secrets encrypts sensitive values at rest (FileVault recovery keys,
// CA and APNs private keys) with AES-256-GCM under a key supplied by the
// operator (VIBEMDM_ENCRYPTION_KEY). Without a key, values are stored
// unencrypted and the server logs a warning at start-up.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

const prefix = "enc:v1:"

// Box seals and opens values.
type Box struct {
	aead cipher.AEAD
}

// New derives a Box from a passphrase (any length; SHA-256 stretched). An empty
// passphrase yields a Box that stores plaintext (Enabled() == false).
func New(passphrase string) (*Box, error) {
	if passphrase == "" {
		return &Box{}, nil
	}
	if len(passphrase) < 16 {
		return nil, errors.New("VIBEMDM_ENCRYPTION_KEY must be at least 16 characters")
	}
	key := sha256.Sum256([]byte(passphrase))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

// Enabled reports whether encryption is active.
func (b *Box) Enabled() bool { return b != nil && b.aead != nil }

// Seal encrypts plaintext for storage. Plaintext passes through when disabled.
func (b *Box) Seal(plaintext string) (string, error) {
	if !b.Enabled() {
		return plaintext, nil
	}
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	ct := b.aead.Seal(nil, nonce, []byte(plaintext), nil)
	return prefix + base64.StdEncoding.EncodeToString(append(nonce, ct...)), nil
}

// Open decrypts a stored value. Values without the prefix are returned as-is
// (legacy plaintext), so enabling encryption later is non-breaking.
func (b *Box) Open(stored string) (string, error) {
	if !strings.HasPrefix(stored, prefix) {
		return stored, nil
	}
	if !b.Enabled() {
		return "", errors.New("value is encrypted but VIBEMDM_ENCRYPTION_KEY is not set")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, prefix))
	if err != nil {
		return "", err
	}
	ns := b.aead.NonceSize()
	if len(raw) < ns {
		return "", errors.New("ciphertext too short")
	}
	pt, err := b.aead.Open(nil, raw[:ns], raw[ns:], nil)
	if err != nil {
		return "", fmt.Errorf("decrypting stored secret: %w", err)
	}
	return string(pt), nil
}

// IsEncrypted reports whether a stored value carries the ciphertext prefix.
func IsEncrypted(stored string) bool { return strings.HasPrefix(stored, prefix) }
