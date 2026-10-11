package agent

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/vibemdm/vibemdm/internal/laps"
	"github.com/vibemdm/vibemdm/internal/localaccount"
	"github.com/vibemdm/vibemdm/internal/model"
)

// Capabilities are the optional features this agent reports at check-in. The server only sends
// a policy that uses one of them to agents that list it.
var Capabilities = []string{"policy_accounts"}

// accountOps runs the Local Accounts steps of a policy (Jamf Pro's Local Accounts payload).
// Passwords are fetched from the server once per run and never logged; every command's output
// is scrubbed of them before it is reported.
type accountOps struct {
	run     laps.Runner
	secrets func(ctx context.Context) ([]string, error)
	lapsCfg func(ctx context.Context) (*model.AgentLAPSConfig, error)
	console func(ctx context.Context) string

	pws     []string
	pwsErr  error
	fetched bool
	admin   *tokenAdmin
}

// tokenAdmin is a Secure Token administrator VibeMDM knows the password of: the LAPS account,
// when it holds a Secure Token (Jamf Pro uses its management account the same way).
type tokenAdmin struct{ user, password string }

func (e *Executor) accountOps(logID string) *accountOps {
	return &accountOps{
		run: RunCommand,
		secrets: func(ctx context.Context) ([]string, error) {
			if logID == "" {
				return nil, fmt.Errorf("this run has no policy log, so its passwords cannot be fetched")
			}
			return e.client.PolicyAccountSecrets(ctx, logID)
		},
		lapsCfg: e.client.LAPSConfig,
		console: ConsoleUser,
	}
}

func (o *accountOps) password(ctx context.Context, i int) (string, error) {
	if !o.fetched {
		o.pws, o.pwsErr = o.secrets(ctx)
		o.fetched = true
	}
	if o.pwsErr != nil {
		return "", fmt.Errorf("fetching the account password from the server: %w", o.pwsErr)
	}
	if i >= len(o.pws) || o.pws[i] == "" {
		return "", fmt.Errorf("the server has no password for this account action")
	}
	return o.pws[i], nil
}

// lapsAccount returns the LAPS account name and, when it holds a Secure Token, its credentials.
func (o *accountOps) lapsAccount(ctx context.Context) (string, *tokenAdmin) {
	if o.lapsCfg == nil {
		return "", nil
	}
	cfg, err := o.lapsCfg(ctx)
	if err != nil || !cfg.Enabled || cfg.Account == "" {
		return "", nil
	}
	if o.admin == nil && cfg.CurrentPassword != "" && laps.HasSecureToken(ctx, o.run, cfg.Account) {
		o.admin = &tokenAdmin{user: cfg.Account, password: cfg.CurrentPassword}
	}
	return cfg.Account, o.admin
}

// Step names for logs: "Create account student".
func accountStepName(a model.PolicyAccount) string {
	verb := map[string]string{localaccount.Create: "Create account", localaccount.ResetPassword: "Reset password for",
		localaccount.Delete: "Delete account", localaccount.DisableFileVault: "Disable FileVault for"}[a.Action]
	return verb + " " + a.Username
}

// apply runs entry i and reports it like any other policy step.
func (o *accountOps) apply(ctx context.Context, i int, a model.PolicyAccount) ExecResult {
	start := time.Now()
	var out strings.Builder
	var secrets []string
	fail := func(format string, args ...any) ExecResult {
		fmt.Fprintf(&out, format, args...)
		return ExecResult{ExitCode: 1, Combined: laps.Scrub(out.String(), secrets...), Duration: time.Since(start)}
	}
	ok := func(format string, args ...any) ExecResult {
		fmt.Fprintf(&out, format, args...)
		return ExecResult{Combined: laps.Scrub(out.String(), secrets...), Duration: time.Since(start)}
	}
	if err := localaccount.Validate(a, true); err != nil {
		return fail("%v", err)
	}
	lapsName, admin := o.lapsAccount(ctx)
	if admin != nil {
		secrets = append(secrets, admin.password)
	}
	if lapsName != "" && strings.EqualFold(a.Username, lapsName) {
		return fail("%s is the LAPS account; it is managed by LAPS, not by policies", a.Username)
	}
	exists := laps.Exists(ctx, o.run, a.Username)

	switch a.Action {
	case localaccount.Create:
		if exists {
			return ok("The account %s already exists; it was left unchanged.", a.Username)
		}
		pw, err := o.password(ctx, i)
		if err != nil {
			return fail("%v", err)
		}
		secrets = append(secrets, pw)
		fullName := a.FullName
		if fullName == "" {
			fullName = a.Username
		}
		args := []string{"-addUser", a.Username, "-fullName", fullName, "-password", pw, "-home", localaccount.HomeDirectory(a)}
		if a.PasswordHint != "" {
			args = append(args, "-hint", a.PasswordHint)
		}
		if a.Admin {
			args = append(args, "-admin")
		}
		res, err := o.run(ctx, "/usr/sbin/sysadminctl", args...)
		out.WriteString(res)
		// sysadminctl often exits 0 when it fails: the account itself is what counts.
		if !laps.Exists(ctx, o.run, a.Username) {
			return fail("\nCreating the account failed%s.", because(err))
		}
		if res, err := o.run(ctx, "/usr/sbin/createhomedir", "-c", "-u", a.Username); err != nil {
			fmt.Fprintf(&out, "\nThe home folder could not be created now (%v); macOS creates it at the first login. %s", err, res)
		}
		role := "a standard user"
		if a.Admin {
			role = "an administrator"
		}
		fmt.Fprintf(&out, "\nCreated %s (%s) as %s, home %s.", a.Username, fullName, role, localaccount.HomeDirectory(a))
		if a.EnableFileVault {
			o.grantToken(ctx, a.Username, pw, admin, &out)
		}
		return ok("")

	case localaccount.ResetPassword:
		if !exists {
			return fail("There is no account named %s.", a.Username)
		}
		pw, err := o.password(ctx, i)
		if err != nil {
			return fail("%v", err)
		}
		secrets = append(secrets, pw)
		if laps.HasSecureToken(ctx, o.run, a.Username) {
			// Changing a Secure Token account's password without its old one needs another Secure
			// Token administrator, or FileVault would keep unlocking with the old password.
			if admin == nil {
				return fail("%s holds a Secure Token, so its password can only be reset with a Secure Token administrator. "+
					"Give the LAPS account a Secure Token (Computers → LAPS → Grant Secure Token) and run the policy again.", a.Username)
			}
			res, err := o.run(ctx, "/usr/sbin/sysadminctl", "-resetPasswordFor", a.Username, "-newPassword", pw, "-adminUser", admin.user, "-adminPassword", admin.password)
			if !laps.Verify(ctx, o.run, a.Username, pw) {
				out.WriteString(res)
				return fail("\nResetting the password failed%s.", because(err))
			}
		} else {
			res, err := o.run(ctx, "/usr/bin/dscl", ".", "-passwd", "/Users/"+a.Username, pw)
			if err != nil || !laps.Verify(ctx, o.run, a.Username, pw) {
				out.WriteString(res)
				return fail("\nResetting the password failed%s.", because(err))
			}
		}
		return ok("Password reset for %s. Their login keychain still opens with the old password; macOS offers to update it at the next login.", a.Username)

	case localaccount.Delete:
		if !exists {
			return ok("There is no account named %s; nothing to delete.", a.Username)
		}
		if strings.EqualFold(o.console(ctx), a.Username) {
			return fail("%s is logged in right now; it was not deleted. The policy can run again later.", a.Username)
		}
		if laps.HasSecureToken(ctx, o.run, a.Username) {
			holders := secureTokenUsers(ctx, o.run)
			if len(holders) <= 1 {
				return fail("%s is the only account with a Secure Token; deleting it would leave nobody able to unlock FileVault or approve system changes.", a.Username)
			}
		}
		args := []string{"-deleteUser", a.Username}
		if !a.DeleteHome {
			args = append(args, "-keepHome")
		}
		res, err := o.run(ctx, "/usr/sbin/sysadminctl", args...)
		out.WriteString(res)
		if laps.Exists(ctx, o.run, a.Username) {
			return fail("\nDeleting the account failed%s.", because(err))
		}
		if a.DeleteHome {
			return ok("\nDeleted %s and its home folder.", a.Username)
		}
		return ok("\nDeleted %s; its home folder was kept.", a.Username)

	case localaccount.DisableFileVault:
		if !exists {
			return fail("There is no account named %s.", a.Username)
		}
		status, _ := o.run(ctx, "/usr/bin/fdesetup", "status")
		if !strings.Contains(status, "FileVault is On") {
			return ok("FileVault is off on this Mac; nothing to do.")
		}
		users := fileVaultUsers(ctx, o.run)
		if !contains(users, a.Username) {
			return ok("%s is not a FileVault user; nothing to do.", a.Username)
		}
		if len(users) <= 1 {
			return fail("%s is the only FileVault user; removing it would leave nobody able to unlock the disk.", a.Username)
		}
		res, err := o.run(ctx, "/usr/bin/fdesetup", "remove", "-user", a.Username)
		out.WriteString(res)
		if contains(fileVaultUsers(ctx, o.run), a.Username) {
			return fail("\nRemoving the account from FileVault failed%s.", because(err))
		}
		return ok("\n%s can no longer unlock FileVault.", a.Username)
	}
	return fail("unknown action %q", a.Action)
}

// grantToken gives a new account a Secure Token (and so FileVault access) using the LAPS
// administrator, like Jamf Pro's "Enable user for FileVault 2" with its management account.
func (o *accountOps) grantToken(ctx context.Context, user, pw string, admin *tokenAdmin, out *strings.Builder) {
	if admin == nil {
		out.WriteString("\nFileVault: no Secure Token administrator is available to VibeMDM (the LAPS account needs a Secure Token). " +
			"With a bootstrap token escrowed, macOS grants the Secure Token when the user first logs in at the login window.")
		return
	}
	o.run(ctx, "/usr/sbin/sysadminctl", "-secureTokenOn", user, "-password", pw, "-adminUser", admin.user, "-adminPassword", admin.password)
	if laps.HasSecureToken(ctx, o.run, user) {
		out.WriteString("\nFileVault: granted a Secure Token, so the user can unlock FileVault.")
		return
	}
	out.WriteString("\nFileVault: granting the Secure Token failed; the account was created without FileVault access.")
}

// secureTokenUsers lists local accounts (UID 501 and up) holding a Secure Token.
func secureTokenUsers(ctx context.Context, run laps.Runner) []string {
	out, err := run(ctx, "/usr/bin/dscl", ".", "-list", "/Users", "UniqueID")
	if err != nil {
		return nil
	}
	var users []string
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) != 2 || strings.HasPrefix(f[0], "_") {
			continue
		}
		if uid, err := strconv.Atoi(f[1]); err != nil || uid < 501 {
			continue
		}
		if laps.HasSecureToken(ctx, run, f[0]) {
			users = append(users, f[0])
		}
	}
	return users
}

// fileVaultUsers lists the users from `fdesetup list` ("name,UUID" per line).
func fileVaultUsers(ctx context.Context, run laps.Runner) []string {
	out, err := run(ctx, "/usr/bin/fdesetup", "list")
	if err != nil {
		return nil
	}
	var users []string
	for _, line := range strings.Split(out, "\n") {
		if name, _, ok := strings.Cut(strings.TrimSpace(line), ","); ok && name != "" {
			users = append(users, name)
		}
	}
	return users
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if strings.EqualFold(x, s) {
			return true
		}
	}
	return false
}

func because(err error) string {
	if err == nil {
		return ""
	}
	return " (" + err.Error() + ")"
}
