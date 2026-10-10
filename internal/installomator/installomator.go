// Package installomator builds safe argument lists for Installomator
// (https://github.com/Installomator/Installomator, Apache-2.0).
//
// Installomator evaluates every "key=value" argument with zsh `eval`, so every
// value is single-quote escaped here and validated against a strict pattern.
// A value can therefore only ever become a literal string assignment.
package installomator

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

// Options are Installomator runtime variables applied to an install.
type Options struct {
	Notify                string `json:"notify"`                  // success | silent | all
	BlockingProcessAction string `json:"blocking_process_action"` // see blockingActions
	Reopen                string `json:"reopen"`                  // yes | no
	IgnoreAppStoreApps    string `json:"ignore_app_store_apps"`   // yes | no
	ForceInstall          bool   `json:"force_install"`           // INSTALL=force
}

var (
	notifyValues    = []string{"success", "silent", "all"}
	blockingActions = []string{"ignore", "silent_fail", "prompt_user", "prompt_user_loop", "prompt_user_then_kill", "tell_user", "tell_user_then_kill", "kill"}
	yesNo           = []string{"yes", "no"}
	// Types supported by Installomator's install flow.
	Types = []string{"dmg", "pkg", "zip", "tbz", "pkgInDmg", "pkgInZip", "appInDmgInZip"}
)

// Choices lists allowed option values for the UI.
func Choices() map[string][]string {
	return map[string][]string{"notify": notifyValues, "blocking_process_action": blockingActions, "reopen": yesNo, "ignore_app_store_apps": yesNo, "type": Types}
}

func oneOf(field, v string, allowed []string) error {
	if v == "" {
		return nil
	}
	for _, a := range allowed {
		if v == a {
			return nil
		}
	}
	return fmt.Errorf("%s must be one of %s", field, strings.Join(allowed, ", "))
}

// Validate checks option values.
func (o Options) Validate() error {
	return errors.Join(oneOf("notify", o.Notify, notifyValues), oneOf("blocking_process_action", o.BlockingProcessAction, blockingActions),
		oneOf("reopen", o.Reopen, yesNo), oneOf("ignore_app_store_apps", o.IgnoreAppStoreApps, yesNo))
}

// CustomLabel defines an app Installomator doesn't ship a label for.
type CustomLabel struct {
	Key               string   `json:"key"`
	Name              string   `json:"name"`
	Type              string   `json:"type"`
	DownloadURL       string   `json:"download_url"`
	ExpectedTeamID    string   `json:"expected_team_id"`
	PackageID         string   `json:"package_id"`
	AppName           string   `json:"app_name"`
	ArchiveName       string   `json:"archive_name"`
	VersionKey        string   `json:"version_key"`
	AppNewVersion     string   `json:"app_new_version"`
	BlockingProcesses []string `json:"blocking_processes"`
}

var (
	keyRe      = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,40}$`)
	teamIDRe   = regexp.MustCompile(`^[A-Z0-9]{10}$`)
	bundleRe   = regexp.MustCompile(`^[A-Za-z0-9._-]{1,200}$`)
	versionRe  = regexp.MustCompile(`^[A-Za-z0-9._-]{0,64}$`)
	textRe     = regexp.MustCompile(`^[^\x00-\x1f\x7f]{1,128}$`) // printable, no control chars/newlines
	fileNameRe = regexp.MustCompile(`^[A-Za-z0-9 ._()\[\]*+-]{1,200}$`)
)

// Validate checks a custom label definition.
func (c CustomLabel) Validate() error {
	var errs []error
	if !keyRe.MatchString(c.Key) {
		errs = append(errs, errors.New("key must be 2–41 lowercase letters, digits, - or _ and start with a letter or digit"))
	}
	if !textRe.MatchString(c.Name) {
		errs = append(errs, errors.New("name is required (printable text, up to 128 characters)"))
	}
	if err := oneOf("type", c.Type, Types); err != nil || c.Type == "" {
		errs = append(errs, fmt.Errorf("type must be one of %s", strings.Join(Types, ", ")))
	}
	u, err := url.Parse(c.DownloadURL)
	if err != nil || u.Scheme != "https" || u.Host == "" || strings.ContainsAny(c.DownloadURL, " \t\r\n'\"`$\\") {
		errs = append(errs, errors.New("download_url must be a plain https:// URL"))
	}
	if !teamIDRe.MatchString(c.ExpectedTeamID) {
		errs = append(errs, errors.New("expected_team_id must be the 10-character Apple Developer Team ID (Installomator verifies the download's signature against it)"))
	}
	if c.PackageID != "" && !bundleRe.MatchString(c.PackageID) {
		errs = append(errs, errors.New("package_id may contain letters, digits, . _ -"))
	}
	if c.AppName != "" && !textRe.MatchString(c.AppName) {
		errs = append(errs, errors.New("app_name must be printable text"))
	}
	if c.ArchiveName != "" && !fileNameRe.MatchString(c.ArchiveName) {
		errs = append(errs, errors.New("archive_name contains unsupported characters"))
	}
	if c.VersionKey != "" && !bundleRe.MatchString(c.VersionKey) {
		errs = append(errs, errors.New("version_key may contain letters, digits, . _ -"))
	}
	if !versionRe.MatchString(c.AppNewVersion) {
		errs = append(errs, errors.New("app_new_version may contain letters, digits, . _ -"))
	}
	for _, p := range c.BlockingProcesses {
		if !textRe.MatchString(p) {
			errs = append(errs, errors.New("blocking process names must be printable text"))
			break
		}
	}
	return errors.Join(errs...)
}

// sq single-quotes a value for zsh so `eval key=<sq(value)>` assigns it literally.
func sq(v string) string { return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'" }

func assign(key, value string) string { return key + "=" + sq(value) }

func (o Options) args() []string {
	var a []string
	notify := o.Notify
	if notify == "" {
		notify = "silent"
	}
	a = append(a, assign("NOTIFY", notify))
	bpa := o.BlockingProcessAction
	if bpa == "" {
		bpa = "prompt_user"
	}
	a = append(a, assign("BLOCKING_PROCESS_ACTION", bpa))
	if o.Reopen != "" {
		a = append(a, assign("REOPEN", o.Reopen))
	}
	if o.IgnoreAppStoreApps != "" {
		a = append(a, assign("IGNORE_APP_STORE_APPS", o.IgnoreAppStoreApps))
	}
	if o.ForceInstall {
		a = append(a, assign("INSTALL", "force"))
	}
	return a
}

// BuiltinArgs returns the argv (after the script path) for a shipped label.
func BuiltinArgs(label string, o Options) ([]string, error) {
	if !keyRe.MatchString(label) && !regexp.MustCompile(`^[a-z0-9][a-zA-Z0-9_-]{0,60}$`).MatchString(label) {
		return nil, fmt.Errorf("invalid label %q", label)
	}
	if err := o.Validate(); err != nil {
		return nil, err
	}
	return append([]string{label}, o.args()...), nil
}

// CustomArgs returns the argv for a custom label via Installomator's
// documented `valuesfromarguments` mechanism.
func CustomArgs(c CustomLabel, o Options) ([]string, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	if err := o.Validate(); err != nil {
		return nil, err
	}
	a := []string{"valuesfromarguments", assign("name", c.Name), assign("type", c.Type), assign("downloadURL", c.DownloadURL), assign("expectedTeamID", c.ExpectedTeamID)}
	opt := map[string]string{"packageID": c.PackageID, "appName": c.AppName, "archiveName": c.ArchiveName, "versionKey": c.VersionKey, "appNewVersion": c.AppNewVersion}
	keys := make([]string, 0, len(opt))
	for k := range opt {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if opt[k] != "" {
			a = append(a, assign(k, opt[k]))
		}
	}
	if len(c.BlockingProcesses) > 0 {
		parts := make([]string, len(c.BlockingProcesses))
		for i, p := range c.BlockingProcesses {
			parts[i] = sq(p)
		}
		a = append(a, "blockingProcesses=( "+strings.Join(parts, " ")+" )")
	}
	return append(a, o.args()...), nil
}
