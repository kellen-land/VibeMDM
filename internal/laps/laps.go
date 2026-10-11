// Package laps holds the pieces of the local administrator password solution
// shared by the server, the agent and the simulator: configuration defaults,
// password generation, and applying a password to a macOS account.
package laps

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"

	"github.com/vibemdm/vibemdm/internal/model"
)

// Defaults and limits for model.LAPSConfig.
const (
	DefaultAccount     = "vibeadmin"
	DefaultLength      = 20
	MinLength          = 14
	MaxLength          = 64
	DefaultRotateDays  = 30
	DefaultAfterUseMin = 60
)

var accountRe = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,30}$`)

// reserved account names the solution must never manage.
var reserved = map[string]bool{"root": true, "daemon": true, "nobody": true, "guest": true, "admin": true}

// Normalize fills defaults and validates a configuration.
func Normalize(c *model.LAPSConfig) error {
	c.AccountName = strings.TrimSpace(c.AccountName)
	if c.AccountName == "" {
		c.AccountName = DefaultAccount
	}
	if !accountRe.MatchString(c.AccountName) || reserved[c.AccountName] {
		return fmt.Errorf("account name %q is not allowed (lowercase letters, digits, - and _, up to 31 characters; not a system account)", c.AccountName)
	}
	if c.PasswordLength == 0 {
		c.PasswordLength = DefaultLength
	}
	if c.PasswordLength < MinLength || c.PasswordLength > MaxLength {
		return fmt.Errorf("password length must be between %d and %d", MinLength, MaxLength)
	}
	if c.RotationDays == 0 {
		c.RotationDays = DefaultRotateDays
	}
	if c.RotationDays < 1 || c.RotationDays > 365 {
		return errors.New("rotation interval must be between 1 and 365 days")
	}
	if c.RotateAfterUseMinutes == 0 {
		c.RotateAfterUseMinutes = DefaultAfterUseMin
	}
	if c.RotateAfterUseMinutes < 5 || c.RotateAfterUseMinutes > 24*60 {
		return errors.New("rotate-after-use delay must be between 5 minutes and 24 hours")
	}
	c.Scope.Normalize()
	return nil
}

// Character classes. Ambiguous characters (0 O 1 l I) are left out so a password
// can be read aloud or typed from a screen.
const (
	lower  = "abcdefghijkmnopqrstuvwxyz"
	upper  = "ABCDEFGHJKLMNPQRSTUVWXYZ"
	digits = "23456789"
	// Symbols avoid anything a shell, plist or dscl parser treats specially.
	symbols = "-_=+.,:@%"
)

// Generate returns a random password of the given length containing every
// character class, using crypto/rand.
func Generate(length int) (string, error) {
	if length < MinLength || length > MaxLength {
		return "", fmt.Errorf("password length must be between %d and %d", MinLength, MaxLength)
	}
	all := lower + upper + digits + symbols
	pick := func(set string) (byte, error) {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(set))))
		if err != nil {
			return 0, err
		}
		return set[n.Int64()], nil
	}
	b := make([]byte, length)
	for i, set := range []string{lower, upper, digits, symbols} { // one of each class
		c, err := pick(set)
		if err != nil {
			return "", err
		}
		b[i] = c
	}
	for i := 4; i < length; i++ {
		c, err := pick(all)
		if err != nil {
			return "", err
		}
		b[i] = c
	}
	for i := length - 1; i > 0; i-- { // Fisher-Yates so the classes are not at fixed positions
		n, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			return "", err
		}
		j := n.Int64()
		b[i], b[j] = b[j], b[i]
	}
	return string(b), nil
}

// Runner executes a command and returns its combined output. The agent supplies
// a real one; tests supply a fake.
type Runner func(ctx context.Context, name string, args ...string) (string, error)

// Exists reports whether the local account exists.
func Exists(ctx context.Context, run Runner, account string) bool {
	_, err := run(ctx, "/usr/bin/dscl", ".", "-read", "/Users/"+account, "UniqueID")
	return err == nil
}

// Verify checks that password is the account's current password.
func Verify(ctx context.Context, run Runner, account, password string) bool {
	_, err := run(ctx, "/usr/bin/dscl", ".", "-authonly", account, password)
	return err == nil
}

// Create creates the account as a hidden-from-login-window administrator.
func Create(ctx context.Context, run Runner, account, password string) error {
	if out, err := run(ctx, "/usr/sbin/sysadminctl", "-addUser", account, "-fullName", "Local Administrator",
		"-password", password, "-admin"); err != nil {
		return fmt.Errorf("creating account: %w: %s", err, Scrub(out, password))
	}
	return nil
}

// SetPassword changes the account password. With the old password it uses
// sysadminctl, which keeps Secure Token and keychain wrapping in step; without it
// (first adoption of an existing account) it falls back to dscl as root.
func SetPassword(ctx context.Context, run Runner, account, oldPassword, newPassword string) error {
	var out string
	var err error
	if oldPassword != "" {
		out, err = run(ctx, "/usr/sbin/sysadminctl", "-resetPasswordFor", account, "-newPassword", newPassword, "-oldPassword", oldPassword)
		if err == nil {
			return nil
		}
	}
	out2, err2 := run(ctx, "/usr/bin/dscl", ".", "-passwd", "/Users/"+account, newPassword)
	if err2 != nil {
		return fmt.Errorf("setting password: %w: %s %s", err2, Scrub(out, oldPassword, newPassword), Scrub(out2, oldPassword, newPassword))
	}
	return nil
}

// Scrub removes secrets from command output before it is logged or reported.
func Scrub(s string, secrets ...string) string {
	for _, sec := range secrets {
		if sec != "" {
			s = strings.ReplaceAll(s, sec, "[redacted]")
		}
	}
	return strings.TrimSpace(s)
}

// Fingerprint identifies a password without revealing any of it: the first
// 8 hex digits of its SHA-256. It is used to match staged and committed passwords.
func Fingerprint(password string) string {
	sum := sha256.Sum256([]byte(password))
	return hex.EncodeToString(sum[:4])
}
