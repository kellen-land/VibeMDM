package installomator

import (
	"os/exec"
	"strings"
	"testing"
)

func validLabel() CustomLabel {
	return CustomLabel{Key: "acme-tool", Name: "Acme Tool", Type: "dmg", DownloadURL: "https://downloads.acme.example/Acme.dmg",
		ExpectedTeamID: "ABCDE12345", AppNewVersion: "2.1.0", BlockingProcesses: []string{"Acme Tool", "Acme Helper"}}
}

func TestValidationRejectsUnsafeInput(t *testing.T) {
	cases := map[string]func(*CustomLabel){
		"http url":        func(c *CustomLabel) { c.DownloadURL = "http://x.example/a.dmg" },
		"command subst":   func(c *CustomLabel) { c.DownloadURL = "https://x.example/$(id).dmg" },
		"quote in url":    func(c *CustomLabel) { c.DownloadURL = "https://x.example/a'.dmg" },
		"bad team":        func(c *CustomLabel) { c.ExpectedTeamID = "abc" },
		"newline in name": func(c *CustomLabel) { c.Name = "a\nrm -rf /" },
		"bad type":        func(c *CustomLabel) { c.Type = "exe" },
		"bad key":         func(c *CustomLabel) { c.Key = "Bad Key" },
		"bad version":     func(c *CustomLabel) { c.AppNewVersion = "1.0;id" },
	}
	for name, mut := range cases {
		c := validLabel()
		mut(&c)
		if err := c.Validate(); err == nil {
			t.Errorf("%s: expected validation error", name)
		}
	}
	if err := validLabel().Validate(); err != nil {
		t.Fatalf("valid label rejected: %v", err)
	}
	if err := (Options{Notify: "loud"}).Validate(); err == nil {
		t.Error("bad notify accepted")
	}
}

// TestEvalIsLiteral proves that what Installomator does — `eval $arg` in zsh,
// or sh as a stand-in — assigns quoted values literally, even hostile ones that
// slip past validation (names may contain $ and backticks).
func TestEvalIsLiteral(t *testing.T) {
	shell, err := exec.LookPath("zsh")
	if err != nil {
		shell, err = exec.LookPath("sh")
		if err != nil {
			t.Skip("no shell")
		}
	}
	hostile := "Evil $(touch /tmp/pwned-vibemdm) `id` '; echo hacked; '"
	arg := assign("name", hostile)
	out, err := exec.Command(shell, "-c", `eval "$1"; printf %s "$name"`, "x", arg).CombinedOutput()
	if err != nil {
		t.Fatalf("eval failed: %v %s", err, out)
	}
	if string(out) != hostile {
		t.Fatalf("value not literal: %q", out)
	}
	bp := "blockingProcesses=( " + sq("A'B") + " " + sq("C $(id)") + " )"
	if strings.Contains(bp, "$(id) )") && !strings.Contains(bp, "'C $(id)'") {
		t.Fatal("array element not quoted")
	}
}

func TestArgs(t *testing.T) {
	a, err := CustomArgs(validLabel(), Options{Notify: "success", ForceInstall: true})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(a, " | ")
	for _, want := range []string{"valuesfromarguments", "name='Acme Tool'", "type='dmg'", "downloadURL='https://downloads.acme.example/Acme.dmg'",
		"expectedTeamID='ABCDE12345'", "appNewVersion='2.1.0'", "blockingProcesses=( 'Acme Tool' 'Acme Helper' )", "NOTIFY='success'", "INSTALL='force'", "DEBUG='0'"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in %s", want, joined)
		}
	}
	b, err := BuiltinArgs("googlechromepkg", Options{})
	// DEBUG=0 always: the bundled script defaults to DEBUG=1, which downloads but never installs.
	if err != nil || b[0] != "googlechromepkg" || b[1] != "DEBUG='0'" || b[2] != "NOTIFY='silent'" {
		t.Fatalf("builtin args: %v %v", b, err)
	}
	if _, err := BuiltinArgs("x; rm -rf /", Options{}); err == nil {
		t.Fatal("hostile builtin label accepted")
	}
}
