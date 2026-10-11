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
	FormatJamf         = "jamf"    // A-Z and 2-9 in groups of five separated by dashes, like Jamf Pro
	FormatComplex      = "complex" // every character class
	DefaultLength      = 30        // Jamf Pro's LAPS passwords are 30 characters
	DefaultComplexLen  = 20
	MinLength          = 14
	MaxLength          = 64
	DefaultRotateDays  = 90 // Jamf Pro's autoRotateExpirationTime default (7776000 s)
	DefaultAfterUseMin = 60 // Jamf Pro's passwordRotationTime default (3600 s)
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
	switch c.PasswordFormat {
	case "":
		c.PasswordFormat = FormatJamf
	case FormatJamf, FormatComplex:
	default:
		return fmt.Errorf("password format must be %q or %q", FormatJamf, FormatComplex)
	}
	if c.PasswordLength == 0 {
		c.PasswordLength = DefaultLength
		if c.PasswordFormat == FormatComplex {
			c.PasswordLength = DefaultComplexLen
		}
	}
	if c.PasswordLength < MinLength || c.PasswordLength > MaxLength {
		return fmt.Errorf("password length must be between %d and %d", MinLength, MaxLength)
	}
	if c.RotationDays == 0 {
		c.RotationDays = DefaultRotateDays
	}
	if c.RotationDays < 1 || c.RotationDays > 3650 {
		return errors.New("rotation interval must be between 1 and 3650 days")
	}
	if c.RotateAfterUseMinutes == 0 {
		c.RotateAfterUseMinutes = DefaultAfterUseMin
	}
	if c.RotateAfterUseMinutes < 1 || c.RotateAfterUseMinutes > 7*24*60 {
		return errors.New("rotation after viewing must be between 1 minute and 7 days")
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

// jamfAlphabet is upper case letters and digits without look-alikes (0 O 1 I).
const jamfAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// GenerateFormat returns a password in the given format. The Jamf format is
// upper case letters and digits in groups of five separated by dashes
// (e.g. "K7QMX-2PZ9R-…"), which Jamf Pro uses because it is easy to read out and
// type at a login window; length counts the dashes.
func GenerateFormat(format string, length int) (string, error) {
	if format != FormatJamf {
		return Generate(length)
	}
	if length < MinLength || length > MaxLength {
		return "", fmt.Errorf("password length must be between %d and %d", MinLength, MaxLength)
	}
	b := make([]byte, length)
	for i := range b {
		if (i+1)%6 == 0 && i != length-1 {
			b[i] = '-'
			continue
		}
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(jamfAlphabet))))
		if err != nil {
			return "", err
		}
		b[i] = jamfAlphabet[n.Int64()]
	}
	return string(b), nil
}

// ValidateManual checks an administrator-chosen password.
func ValidateManual(pw string) error {
	if len(pw) < 8 || len(pw) > 128 {
		return errors.New("the password must be 8 to 128 characters")
	}
	if strings.TrimSpace(pw) != pw || strings.ContainsAny(pw, "\x00\r\n\t") {
		return errors.New("the password must not contain control characters or leading/trailing spaces")
	}
	return nil
}

// Hide hides the account from the login window and Users & Groups.
func Hide(ctx context.Context, run Runner, account string) error {
	_, err := run(ctx, "/usr/bin/dscl", ".", "-create", "/Users/"+account, "IsHidden", "1")
	return err
}

// GUID returns the account's GeneratedUID ("" if it cannot be read).
func GUID(ctx context.Context, run Runner, account string) string {
	out, err := run(ctx, "/usr/bin/dscl", ".", "-read", "/Users/"+account, "GeneratedUID")
	if err != nil {
		return ""
	}
	f := strings.Fields(out)
	if len(f) >= 2 && f[0] == "GeneratedUID:" {
		return f[1]
	}
	return ""
}
