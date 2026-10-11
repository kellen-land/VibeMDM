// Package localaccount validates the Local Accounts policy payload (Jamf Pro's Create Account,
// Reset Password, Delete Account and Disable Account for FileVault) on the server and the agent.
package localaccount

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/vibemdm/vibemdm/internal/model"
)

// Actions.
const (
	Create           = "create"
	ResetPassword    = "reset_password"
	Delete           = "delete"
	DisableFileVault = "disable_filevault"
)

// usernameRe is a macOS short name: no leading underscore (system accounts), 31 characters at most.
var usernameRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]{0,30}$`)

var reserved = map[string]bool{"root": true, "daemon": true, "nobody": true, "guest": true}

// NeedsPassword reports whether the action carries a password.
func NeedsPassword(action string) bool { return action == Create || action == ResetPassword }

// HomeDirectory returns the account's home folder (default /Users/<username>).
func HomeDirectory(a model.PolicyAccount) string {
	if a.HomeDirectory != "" {
		return a.HomeDirectory
	}
	return "/Users/" + a.Username
}

// Validate checks one entry. hasPassword is true when a password is sent or already stored.
func Validate(a model.PolicyAccount, hasPassword bool) error {
	switch a.Action {
	case Create, ResetPassword, Delete, DisableFileVault:
	default:
		return fmt.Errorf("unknown local account action %q", a.Action)
	}
	if !usernameRe.MatchString(a.Username) || reserved[strings.ToLower(a.Username)] {
		return fmt.Errorf("username %q is not allowed (letters, digits, . _ -, starting with a letter, up to 31 characters; not a system account)", a.Username)
	}
	if NeedsPassword(a.Action) {
		if !hasPassword {
			return fmt.Errorf("%s: a password is required", a.Username)
		}
		if a.Password != "" {
			if err := validPassword(a.Password); err != nil {
				return fmt.Errorf("%s: %w", a.Username, err)
			}
		}
	}
	if a.Action != Create {
		return nil
	}
	if len(a.FullName) > 128 || !printable(a.FullName) {
		return fmt.Errorf("%s: the full name must be printable text up to 128 characters", a.Username)
	}
	if len(a.PasswordHint) > 200 || !printable(a.PasswordHint) {
		return fmt.Errorf("%s: the password hint must be printable text up to 200 characters", a.Username)
	}
	if a.Password != "" && a.PasswordHint != "" && strings.Contains(strings.ToLower(a.PasswordHint), strings.ToLower(a.Password)) {
		return fmt.Errorf("%s: the password hint must not contain the password", a.Username)
	}
	if h := a.HomeDirectory; h != "" {
		if !strings.HasPrefix(h, "/") || strings.Contains(h, "/../") || strings.HasSuffix(h, "/..") || len(h) > 255 || !printable(h) || h == "/" {
			return fmt.Errorf("%s: the home directory must be an absolute path such as /Users/%s", a.Username, a.Username)
		}
	}
	return nil
}

func validPassword(pw string) error {
	if len(pw) < 4 || len(pw) > 128 {
		return errors.New("the password must be 4 to 128 characters")
	}
	if strings.TrimSpace(pw) != pw || !printable(pw) {
		return errors.New("the password must not contain control characters or leading/trailing spaces")
	}
	return nil
}

func printable(s string) bool {
	for _, r := range s {
		if !unicode.IsPrint(r) {
			return false
		}
	}
	return true
}
