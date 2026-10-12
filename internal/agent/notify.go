package agent

import (
	"context"
	"fmt"
	"net/url"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/vibemdm/vibemdm/internal/model"
)

// Notification is a macOS notification for the logged-in user.
type Notification struct {
	Title, Message string
	// Icon is an image file or an .app bundle shown in the notification (swiftDialog only).
	Icon string
	// Open is an .app bundle opened when the notification is clicked (swiftDialog only).
	Open string
}

// PostNotification shows a notification to the console user. swiftDialog posts it when it is
// installed (it installs with Self Service): it can show an image and choose what a click
// opens. Without it, osascript posts it, which macOS shows with Script Editor's icon and opens
// Script Editor when clicked.
func PostNotification(ctx context.Context, n Notification) {
	user := ConsoleUser(ctx)
	if user == "" || user == "loginwindow" || n.Message == "" {
		return
	}
	uid := fmt.Sprint(ConsoleUserUID(ctx, user))
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if fileExists(dialogBinary) {
		args := []string{"asuser", uid, "sudo", "-u", user, dialogBinary, "--notification", "--title", n.Title, "--message", n.Message}
		if n.Icon != "" && fileExists(n.Icon) {
			args = append(args, "--icon", n.Icon)
		}
		if n.Open != "" && fileExists(n.Open) {
			args = append(args, "--button1action", (&url.URL{Scheme: "file", Path: n.Open}).String())
		}
		if exec.CommandContext(ctx, "launchctl", args...).Run() == nil {
			return
		}
	}
	script := fmt.Sprintf(`display notification %q with title %q`, n.Message, n.Title)
	exec.CommandContext(ctx, "launchctl", "asuser", uid, "sudo", "-u", user, "osascript", "-e", script).Run()
}

// appRun is one Installomator app install (catalog install, policy step or Self Service).
type appRun struct {
	AppID string // catalog app (for its uploaded icon); empty for raw labels
	Name  string
	Label string
	Args  []string
}

// installApp runs Installomator for an app with its own notifications turned off and posts the
// result notification itself, with the app's icon, when the app's Notify option asks for one.
// Installomator's notifications use osascript (Script Editor's icon, and clicking opens Script
// Editor); arguments are evaluated in order, so the appended NOTIFY=silent wins.
func (e *Executor) installApp(ctx context.Context, script string, a appRun) ExecResult {
	mode := notifyMode(a.Args)
	args := a.Args
	if mode != "silent" && len(args) > 0 {
		args = append(append([]string{}, args...), "NOTIFY=silent")
	}
	r := e.runInstallomator(ctx, script, a.Label, args)
	if n, ok := e.appNotification(ctx, a, mode, r); ok {
		PostNotification(ctx, n)
	}
	return r
}

// notifyMode is the NOTIFY value the server asked for (Installomator's default: success).
func notifyMode(args []string) string {
	mode := "success"
	for _, a := range args {
		if v, ok := strings.CutPrefix(a, "NOTIFY="); ok {
			mode = strings.Trim(v, `'"`)
		}
	}
	return mode
}

var (
	// finishing() logs "Installed <name>[, version <v>]" at level REQ.
	installedLineRe = regexp.MustCompile(`(?m): REQ\s*: [^:\n]* : (Installed .+?)\s*$`)
	foundAppRe      = regexp.MustCompile(`found app at (/.+?\.app), version`)
)

// appNotification builds the notification Installomator would have shown for mode
// (success: installed or updated; all: also up to date and failures).
func (e *Executor) appNotification(ctx context.Context, a appRun, mode string, r ExecResult) (Notification, bool) {
	if mode != "success" && mode != "all" {
		return Notification{}, false
	}
	name := firstNonEmpty(a.Name, a.Label)
	out := r.Combined
	n := Notification{}
	switch {
	case r.ExitCode == 0 && installedLineRe.MatchString(out):
		m := installedLineRe.FindAllStringSubmatch(out, -1)
		n.Title, n.Message = name+" is ready", strings.TrimSpace(m[len(m)-1][1])
		// The app is found once before installing when it was already there, and once after.
		if len(foundAppRe.FindAllString(out, -1)) > 1 {
			n.Title = name + " was updated"
		}
	case r.ExitCode == 0 && mode == "all":
		n.Title, n.Message = name, name+" is already up to date."
	case r.ExitCode != 0 && mode == "all":
		n.Title, n.Message = name+" could not be installed", "Try again from Self Service, or contact IT if it keeps failing."
	default:
		return Notification{}, false
	}
	if m := foundAppRe.FindAllStringSubmatch(out, -1); len(m) > 0 {
		n.Open = m[len(m)-1][1]
	}
	if a.AppID != "" {
		if p, err := uploadedAppIcon(ctx, e.client, a.AppID, ""); err == nil {
			n.Icon = p
		}
	}
	if n.Icon == "" {
		n.Icon = n.Open // swiftDialog shows an app bundle's own icon
	}
	return n, true
}

// appRunFromStep converts a policy app step.
func appRunFromStep(s model.AgentAppStep) appRun {
	return appRun{AppID: s.AppID, Name: s.Name, Label: s.Label, Args: s.Args}
}
