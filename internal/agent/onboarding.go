package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/vibemdm/vibemdm/internal/model"
)

// swiftDialog (MIT, github.com/swiftDialog/swiftDialog) drives the onboarding UI.
const (
	dialogBinary      = "/usr/local/bin/dialog"
	dialogCommandFile = "/var/tmp/vibemdm-onboarding.log"
)

// dialogSafe removes characters swiftDialog treats as separators in option strings.
func dialogSafe(s string) string {
	return strings.NewReplacer(",", " ", "\n", " ", "\r", " ").Replace(strings.TrimSpace(s))
}

func (e *Executor) ensureDialog(ctx context.Context, args []string) error {
	if _, err := os.Stat(dialogBinary); err == nil {
		return nil
	}
	script, err := e.ensureInstallomator(ctx)
	if err != nil {
		return err
	}
	if len(args) == 0 {
		args = []string{"swiftdialog", "NOTIFY=silent"}
	}
	if r := e.runInstallomator(ctx, script, "swiftdialog", args); r.ExitCode != 0 {
		return fmt.Errorf("installing swiftDialog failed (exit %d)", r.ExitCode)
	}
	if _, err := os.Stat(dialogBinary); err != nil {
		return fmt.Errorf("swiftDialog is not available after installation")
	}
	return nil
}

func commonDialogArgs(p model.AgentOnboarding) []string {
	args := []string{"--title", p.Title, "--ontop", "--moveable", "--width", "820", "--height", "600"}
	if p.Message != "" {
		args = append(args, "--message", p.Message)
	}
	if p.Icon != "" {
		args = append(args, "--icon", p.Icon)
	}
	if p.Banner != "" {
		args = append(args, "--bannerimage", p.Banner)
	}
	if p.BlurScreen {
		args = append(args, "--blurscreen")
	}
	return args
}

// collectUserEntry shows the welcome form and returns the values entered.
func collectUserEntry(ctx context.Context, p model.AgentOnboarding) (map[string]string, error) {
	if len(p.Fields) == 0 {
		return map[string]string{}, nil
	}
	args := append(commonDialogArgs(p), "--json", "--button1text", "Continue")
	selects := map[string]string{} // dialog title -> field key
	for _, f := range p.Fields {
		label := dialogSafe(f.Label)
		if len(f.Options) > 0 {
			opt := label
			if f.Required {
				opt += ",required"
			}
			vals := make([]string, 0, len(f.Options))
			for _, o := range f.Options {
				vals = append(vals, dialogSafe(o))
			}
			args = append(args, "--selecttitle", opt, "--selectvalues", strings.Join(vals, ","))
			selects[label] = f.Key
			continue
		}
		opt := label + ",name=" + f.Key
		if f.Required {
			opt += ",required"
		}
		if f.Placeholder != "" {
			opt += ",prompt=" + dialogSafe(f.Placeholder)
		}
		if f.Regex != "" && !strings.Contains(f.Regex, ",") {
			opt += ",regex=" + f.Regex
			if f.RegexError != "" {
				opt += ",regexerror=" + dialogSafe(f.RegexError)
			}
		}
		args = append(args, "--textfield", opt)
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, dialogBinary, args...).Output()
	if err != nil && len(out) == 0 {
		return nil, fmt.Errorf("welcome dialog: %w", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("welcome dialog returned no values")
	}
	entry := map[string]string{}
	for k, v := range raw {
		key := k
		if fk, ok := selects[k]; ok {
			key = fk
		}
		switch t := v.(type) {
		case string:
			entry[key] = t
		case map[string]any: // select output: {"selectedValue": ..., "selectedIndex": ...}
			for _, sk := range []string{"selectedValue", "SelectedOption"} {
				if s, ok := t[sk].(string); ok {
					entry[key] = s
				}
			}
		}
	}
	// Keep only keys the workflow asked for.
	clean := map[string]string{}
	for _, f := range p.Fields {
		if v, ok := entry[f.Key]; ok {
			clean[f.Key] = strings.TrimSpace(v)
		}
	}
	return clean, nil
}

var hostSafe = regexp.MustCompile(`[^A-Za-z0-9-]+`)

// computerName renders the naming template ({serial}, {asset_tag}, {user}, {model}).
func computerName(tmpl string, entry map[string]string, si SystemInfo) string {
	if v := entry["computer_name"]; v != "" {
		return v
	}
	if tmpl == "" {
		return ""
	}
	r := strings.NewReplacer("{serial}", si.SerialNumber, "{asset_tag}", entry["asset_tag"], "{user}", firstNonEmpty(entry["assigned_user"], si.ConsoleUser),
		"{model}", modelShort(si.ModelIdentifier))
	return strings.TrimSpace(r.Replace(tmpl))
}

// modelShort turns "MacBookPro18,3" into "MBP".
func modelShort(id string) string {
	for _, m := range []struct{ prefix, short string }{{"MacBookPro", "MBP"}, {"MacBookAir", "MBA"}, {"Macmini", "MINI"}, {"iMac", "IMAC"},
		{"MacPro", "MP"}, {"Mac", "MAC"}} {
		if strings.HasPrefix(id, m.prefix) {
			return m.short
		}
	}
	return "MAC"
}

func setComputerName(ctx context.Context, name string) {
	if name == "" {
		return
	}
	host := strings.Trim(hostSafe.ReplaceAllString(name, "-"), "-")
	if len(host) > 63 {
		host = host[:63]
	}
	exec.CommandContext(ctx, "/usr/sbin/scutil", "--set", "ComputerName", name).Run()
	if host != "" {
		exec.CommandContext(ctx, "/usr/sbin/scutil", "--set", "LocalHostName", host).Run()
		exec.CommandContext(ctx, "/usr/sbin/scutil", "--set", "HostName", host).Run()
	}
}

// progressDialog drives a running swiftDialog through its command file.
type progressDialog struct {
	cmd  *exec.Cmd
	file *os.File
	done chan struct{}
}

func startProgressDialog(ctx context.Context, p model.AgentOnboarding) (*progressDialog, error) {
	os.Remove(dialogCommandFile) // never follow a planted symlink
	f, err := os.OpenFile(dialogCommandFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return nil, err
	}
	args := append(commonDialogArgs(p), "--commandfile", dialogCommandFile, "--progress", fmt.Sprint(len(p.Steps)),
		"--button1text", "Please wait", "--button1disabled", "--liststyle", "compact")
	for _, s := range p.Steps {
		args = append(args, "--listitem", dialogSafe(s.Title))
	}
	cmd := exec.CommandContext(ctx, dialogBinary, args...)
	if err := cmd.Start(); err != nil {
		f.Close()
		return nil, err
	}
	d := &progressDialog{cmd: cmd, file: f, done: make(chan struct{})}
	go func() { cmd.Wait(); close(d.done) }()
	time.Sleep(500 * time.Millisecond)
	return d, nil
}

func (d *progressDialog) send(format string, a ...any) {
	if d == nil {
		return
	}
	fmt.Fprintf(d.file, format+"\n", a...)
}

func (d *progressDialog) item(i int, status, text string) {
	d.send("listitem: index: %d, status: %s, statustext: %s", i, status, dialogSafe(text))
}

// finish enables the button and waits (up to 10 minutes) for the user to close the window.
func (d *progressDialog) finish(button string) {
	if d == nil {
		return
	}
	d.send("button1text: %s", button)
	d.send("button1: enable")
	select {
	case <-d.done:
	case <-time.After(10 * time.Minute):
		d.send("quit:")
	}
	d.file.Close()
	os.Remove(dialogCommandFile)
}

// RunOnboarding executes a white-glove onboarding workflow.
func (e *Executor) RunOnboarding(ctx context.Context, p model.AgentOnboarding) ExecResult {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Hour)
	defer cancel()
	var logb strings.Builder
	logf := func(format string, a ...any) {
		fmt.Fprintf(&logb, time.Now().Format("15:04:05")+" "+format+"\n", a...)
	}
	sub := model.AgentOnboardingSubmit{Status: "completed", UserEntry: map[string]string{}}
	si := CollectSystemInfo(ctx)
	ui := si.ConsoleUser != "" && si.ConsoleUser != "root"
	if ui {
		if err := e.ensureDialog(ctx, p.DialogArgs); err != nil {
			logf("swiftDialog unavailable (%v); continuing without a window", err)
			ui = false
		}
	} else {
		logf("no user logged in; running steps without a window")
	}

	if ui && len(p.Fields) > 0 {
		entry, err := collectUserEntry(ctx, p)
		if err != nil {
			logf("%v", err)
		} else {
			sub.UserEntry = entry
			logf("collected %d value(s) from the user", len(entry))
		}
	}
	if name := computerName(p.ComputerNameTemplate, sub.UserEntry, si); name != "" {
		setComputerName(ctx, name)
		sub.UserEntry["computer_name"] = name
		logf("computer name set to %q", name)
	}
	if err := e.client.OnboardingUserEntry(ctx, p.RunID, sub.UserEntry); err != nil {
		logf("sending user entry: %v", err)
	}

	var dlg *progressDialog
	if ui && len(p.Steps) > 0 {
		var err error
		if dlg, err = startProgressDialog(ctx, p); err != nil {
			logf("progress window: %v", err)
		}
	}
	for i, s := range p.Steps {
		dlg.item(i, "wait", "Working…")
		dlg.send("progresstext: %s", dialogSafe(s.Title))
		start := time.Now()
		r := e.runOnboardingStep(ctx, s)
		res := model.OnboardingStepResult{Title: s.Title, Type: s.Type, Status: "success", DurationMS: time.Since(start).Milliseconds()}
		if r.ExitCode != 0 || r.Err != nil {
			res.Status = "failed"
			res.Detail = strings.TrimSpace(firstNonEmpty(errString(r.Err), lastLines(r.Combined, 3)))
			dlg.item(i, "fail", "Failed")
			logf("step %q failed: %s", s.Title, res.Detail)
		} else {
			dlg.item(i, "success", "Done")
			logf("step %q done in %s", s.Title, time.Since(start).Round(time.Second))
		}
		sub.Steps = append(sub.Steps, res)
		dlg.send("progress: %d", i+1)
		if res.Status == "failed" && !s.ContinueOnError {
			sub.Status = "failed"
			for j := i + 1; j < len(p.Steps); j++ {
				sub.Steps = append(sub.Steps, model.OnboardingStepResult{Title: p.Steps[j].Title, Type: p.Steps[j].Type, Status: "skipped"})
				dlg.item(j, "error", "Skipped")
			}
			break
		}
	}
	finalMsg := p.FinalMessage
	if finalMsg == "" {
		finalMsg = "Your Mac is ready."
	}
	if sub.Status == "failed" {
		finalMsg = "Some items could not be completed. IT has been notified."
	}
	button := "Done"
	if sub.Status == "completed" {
		switch p.FinalAction {
		case "restart":
			button = "Restart"
		case "logout":
			button = "Log Out"
		case "shutdown":
			button = "Shut Down"
		}
	}
	dlg.send("progresstext: %s", dialogSafe(finalMsg))
	dlg.send("message: %s", finalMsg)
	dlg.finish(button)

	sub.Log = logb.String()
	sctx, scancel := context.WithTimeout(context.Background(), time.Minute)
	defer scancel()
	if err := e.client.SubmitOnboarding(sctx, p.RunID, sub); err != nil {
		e.log.Error("submitting onboarding result", "err", err)
	}
	if sub.Status == "completed" {
		switch p.FinalAction {
		case "restart":
			exec.Command("/sbin/shutdown", "-r", "+1").Start()
		case "shutdown":
			exec.Command("/sbin/shutdown", "-h", "+1").Start()
		case "logout":
			if u := ConsoleUser(ctx); u != "" {
				exec.Command("launchctl", "bootout", fmt.Sprintf("gui/%d", ConsoleUserUID(ctx, u))).Start()
			}
		}
	}
	summary := fmt.Sprintf("onboarding %s: %d step(s)", sub.Status, len(sub.Steps))
	if sub.Status == "failed" {
		return ExecResult{ExitCode: 1, Stdout: summary, Combined: sub.Log}
	}
	return ExecResult{Stdout: summary, Combined: sub.Log}
}

func (e *Executor) runOnboardingStep(ctx context.Context, s model.AgentOnboardingStep) ExecResult {
	switch s.Type {
	case "app":
		if s.App == nil {
			return ExecResult{ExitCode: 1, Err: fmt.Errorf("missing app")}
		}
		return e.RunAppStep(ctx, *s.App)
	case "policy":
		if s.Policy == nil {
			return ExecResult{ExitCode: 1, Err: fmt.Errorf("missing policy")}
		}
		sub := e.RunPolicy(ctx, *s.Policy)
		if sub.Status != "completed" {
			return ExecResult{ExitCode: 1, Combined: sub.Log}
		}
		return ExecResult{Combined: sub.Log}
	case "script":
		return e.RunScript(ctx, s.ScriptName, s.ScriptContents, nil, nil)
	case "recon":
		if err := e.Recon(ctx); err != nil {
			return ExecResult{ExitCode: 1, Err: err}
		}
		return ExecResult{}
	case "wait":
		select {
		case <-time.After(time.Duration(s.Seconds) * time.Second):
		case <-ctx.Done():
		}
		return ExecResult{}
	}
	return ExecResult{ExitCode: 1, Err: fmt.Errorf("unknown step type %q", s.Type)}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, " | ")
}
