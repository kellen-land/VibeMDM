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
	if strings.ContainsRune(symbols, rune(b[0])) || hasSimpleRun(string(b)) {
		return Generate(length) // a leading "-" reads as an option; runs fail "no simple passwords" policies
	}
	return string(b), nil
}

// hasSimpleRun reports three repeated (aaa) or consecutive (abc, 321) characters, which
// macOS password policies that disallow simple passwords reject.
func hasSimpleRun(s string) bool {
	for i := 2; i < len(s); i++ {
		a, b, c := int(s[i-2]), int(s[i-1]), int(s[i])
		if (a == b && b == c) || (b-a == 1 && c-b == 1) || (a-b == 1 && b-c == 1) {
			return true
		}
	}
	return false
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
	out, err := run(ctx, "/usr/sbin/sysadminctl", "-addUser", account, "-fullName", "Local Administrator", "-password", password, "-admin")
	// sysadminctl often exits 0 when it fails, so the account itself is what counts.
	if !Exists(ctx, run, account) {
		return &NotAppliedError{toolError("creating the account", err, out, password)}
	}
	if Verify(ctx, run, account, password) {
		return nil
	}
	return toolError("creating the account", err, out, password)
}

// NotAppliedError is a failure where the account is known to be unchanged, so the
// new password can safely be discarded. Any other error leaves the outcome open.
type NotAppliedError struct{ Err error }

func (e *NotAppliedError) Error() string { return e.Err.Error() }
func (e *NotAppliedError) Unwrap() error { return e.Err }

// SetPassword changes the account password and confirms it by authenticating with it.
//
// An account without a Secure Token has no FileVault or Secure Token state to keep in step,
// so root sets the password with dscl directly: no sign-in attempt is made with the recorded
// old password, which, if stale, would count as a failed login and under a lockout policy
// (mSCP/STIG: 3 attempts) lock the account. A Secure Token account must be changed with
// sysadminctl and its old password, so FileVault and the keychain stay in step; dscl is
// never used on it.
//
// sysadminctl regularly exits 0 when it did not change anything (wrong old password,
// password policy), so the result is judged by authenticating with the new password. A
// *NotAppliedError means the account is unchanged; any other error means the outcome could
// not be confirmed (for example the account is locked out, so even a correct password fails).
func SetPassword(ctx context.Context, run Runner, account, oldPassword, newPassword string) error {
	if !HasSecureToken(ctx, run, account) {
		out, err := run(ctx, "/usr/bin/dscl", ".", "-passwd", "/Users/"+account, newPassword)
		if err != nil {
			return &NotAppliedError{toolError("setting the password", err, out, oldPassword, newPassword)}
		}
		if Verify(ctx, run, account, newPassword) {
			return nil
		}
		return toolError("setting the password", nil, "", oldPassword, newPassword)
	}
	if oldPassword == "" {
		return &NotAppliedError{errors.New("the account holds a Secure Token and its current password is unknown, so it cannot be changed safely; set the password with Set password in VibeMDM or reset it with a Secure Token administrator")}
	}
	out, err := run(ctx, "/usr/sbin/sysadminctl", "-resetPasswordFor", account, "-newPassword", newPassword, "-oldPassword", oldPassword)
	if Verify(ctx, run, account, newPassword) {
		return nil // also when the tool exited non-zero after the change took effect
	}
	e := toolError("sysadminctl could not change the password of this Secure Token account", err, out, oldPassword, newPassword)
	if err != nil || explainToolOutput(Scrub(out, oldPassword, newPassword)) != "" {
		return &NotAppliedError{e}
	}
	return e
}

// LockedOut reports whether the Mac's password policy currently refuses sign-ins for the
// account (too many failed attempts), from `pwpolicy -u <account> -authentication-allowed`.
// known is false when the answer is not recognised.
func LockedOut(ctx context.Context, run Runner, account string) (locked, known bool) {
	out, _ := run(ctx, "/usr/bin/pwpolicy", "-u", account, "-authentication-allowed")
	return parseAuthAllowed(out)
}

func parseAuthAllowed(out string) (locked, known bool) {
	o := strings.ToLower(out)
	for _, no := range []string{"not allow", "disallow", "denies", "denied", "forbid", "does not permit", "not permitted"} {
		if strings.Contains(o, no) {
			return true, true
		}
	}
	if strings.Contains(o, "allows") || strings.Contains(o, "is allowed") || strings.Contains(o, "permits") {
		return false, true
	}
	return false, false
}

// toolError reports a failed sysadminctl/dscl call with its (scrubbed) output and, when the
// output is recognisable, a plain explanation.
func toolError(what string, err error, out string, secrets ...string) error {
	msg := Scrub(out, secrets...)
	if hint := explainToolOutput(msg); hint != "" {
		msg = hint + " (" + firstLine(msg) + ")"
	}
	switch {
	case msg == "" && err != nil:
		return fmt.Errorf("%s failed: %w", what, err)
	case msg == "":
		return fmt.Errorf("%s failed: the new password did not authenticate afterwards", what)
	}
	return fmt.Errorf("%s failed: %s", what, msg)
}

// explainToolOutput turns well-known Directory Services errors into advice.
func explainToolOutput(out string) string {
	o := strings.ToLower(out)
	switch {
	case strings.Contains(o, "-14165") || strings.Contains(o, "quality") || strings.Contains(o, "policy") || strings.Contains(o, "requirement"):
		return "the Mac's password policy rejected the new password; a configuration profile (for example an mSCP baseline) sets the rules, so choose a LAPS password format and length that meet them"
	case strings.Contains(o, "-14090") || strings.Contains(o, "authentication") || strings.Contains(o, "old password"):
		return "the current password VibeMDM has on record is not the account's password; use Set password to give it a new one"
	case strings.Contains(o, "secure token") || strings.Contains(o, "not permitted"):
		return "macOS refused the change because of the account's Secure Token"
	}
	return ""
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}

// HasSecureToken reports whether the account holds a Secure Token
// (sysadminctl prints "Secure token is ENABLED for user …").
func HasSecureToken(ctx context.Context, run Runner, account string) bool {
	out, err := run(ctx, "/usr/sbin/sysadminctl", "-secureTokenStatus", account)
	return err == nil && strings.Contains(out, "ENABLED")
}

// VolumeOwner reports whether the account (by GeneratedUID) is a volume owner of the system
// volume, from `diskutil apfs listUsers /`. nil means unknown or not applicable (Intel Macs
// have no volume owners).
func VolumeOwner(ctx context.Context, run Runner, guid string) *bool {
	if guid == "" {
		return nil
	}
	out, err := run(ctx, "/usr/sbin/diskutil", "apfs", "listUsers", "/")
	if err != nil {
		return nil
	}
	return parseVolumeOwner(out, guid)
}

func parseVolumeOwner(out, guid string) *bool {
	inUser := false
	for _, line := range strings.Split(out, "\n") {
		t := strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "|+- "))
		if strings.EqualFold(t, guid) {
			inUser = true
			continue
		}
		if !inUser {
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(line), "+--") {
			return nil // next user: this one had no Volume Owner line
		}
		if v, ok := strings.CutPrefix(t, "Volume Owner:"); ok {
			yes := strings.EqualFold(strings.TrimSpace(v), "Yes")
			return &yes
		}
	}
	return nil
}

// GrantSecureToken gives the account a Secure Token using an existing Secure Token
// administrator's credentials. On Apple silicon, when that administrator is a volume owner,
// the account also becomes a volume owner. Apple offers no way to do this without such
// credentials (other than the bootstrap token at the account's first login-window sign-in).
func GrantSecureToken(ctx context.Context, run Runner, account, password, adminUser, adminPassword string) error {
	out, err := run(ctx, "/usr/sbin/sysadminctl", "-secureTokenOn", account, "-password", password, "-adminUser", adminUser, "-adminPassword", adminPassword)
	// sysadminctl often exits 0 even on failure; the status check is what counts.
	if HasSecureToken(ctx, run, account) {
		return nil
	}
	msg := Scrub(out, password, adminPassword)
	if err != nil {
		return fmt.Errorf("granting the Secure Token failed: %w: %s", err, msg)
	}
	return fmt.Errorf("granting the Secure Token failed: %s", msg)
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
	if hasSimpleRun(string(b)) {
		return GenerateFormat(format, length)
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
