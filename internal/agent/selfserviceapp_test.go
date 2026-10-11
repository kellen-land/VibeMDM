package agent

import (
	"bytes"
	"context"
	"encoding/binary"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vibemdm/vibemdm/internal/model"
)

func newTestSSApp(t *testing.T) *selfServiceApp {
	dir := t.TempDir()
	return &selfServiceApp{appsDir: filepath.Join(dir, "Applications"), statePath: filepath.Join(dir, "state.json"), version: "v1", log: nopLog{}}
}

func TestSelfServiceAppLifecycle(t *testing.T) {
	s := newTestSSApp(t)
	os.MkdirAll(s.appsDir, 0o755)
	ctx := context.Background()
	s.cfg = model.AgentSelfService{Enabled: true, OrgName: "Lands School", Accent: "#3366CC"}
	if err := s.reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	app := filepath.Join(s.appsDir, "Lands School Self Service.app")
	exe := filepath.Join(app, "Contents", "MacOS", "selfservice")
	fi, err := os.Stat(exe)
	if err != nil || fi.Mode().Perm()&0o111 == 0 {
		t.Fatalf("launcher missing or not executable: %v", err)
	}
	plist, _ := os.ReadFile(filepath.Join(app, "Contents", "Info.plist"))
	if !strings.Contains(string(plist), "<string>Lands School Self Service</string>") || !strings.Contains(string(plist), selfServiceBundleID) {
		t.Fatalf("Info.plist: %s", plist)
	}
	// Unchanged configuration: nothing is rewritten.
	before, _ := os.Stat(exe)
	if err := s.reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if after, _ := os.Stat(exe); !after.ModTime().Equal(before.ModTime()) {
		t.Fatal("an up-to-date app was rewritten")
	}
	// Renamed in the settings: the old bundle goes, the new one appears.
	s.cfg.AppName = "Lands Apps"
	if err := s.reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if fileExists(app) || !fileExists(filepath.Join(s.appsDir, "Lands Apps.app", "Contents", "Info.plist")) {
		t.Fatal("rename not applied")
	}
	// Turned off: removed.
	s.cfg.Enabled = false
	s.reconcile(ctx)
	if fileExists(filepath.Join(s.appsDir, "Lands Apps.app")) {
		t.Fatal("app not removed when Self Service is off")
	}
}

func TestSelfServiceAppNeverReplacesSomeoneElsesApp(t *testing.T) {
	s := newTestSSApp(t)
	foreign := filepath.Join(s.appsDir, "Acme Self Service.app", "Contents")
	os.MkdirAll(foreign, 0o755)
	os.WriteFile(filepath.Join(foreign, "Info.plist"), []byte("theirs"), 0o644)
	s.cfg = model.AgentSelfService{Enabled: true, OrgName: "Acme"}
	if err := s.reconcile(context.Background()); err == nil {
		t.Fatal("expected a refusal")
	}
	if b, _ := os.ReadFile(filepath.Join(foreign, "Info.plist")); string(b) != "theirs" {
		t.Fatal("overwrote an app VibeMDM did not create")
	}
}

func TestSelfServiceIconIsAValidICNS(t *testing.T) {
	b := selfServiceIcon("#0FB5AE")
	if string(b[:4]) != "icns" || int(binary.BigEndian.Uint32(b[4:8])) != len(b) {
		t.Fatal("bad icns header")
	}
	// First entry: ic07, a 128-pixel PNG.
	if string(b[8:12]) != "ic07" {
		t.Fatalf("first entry %q", b[8:12])
	}
	n := int(binary.BigEndian.Uint32(b[12:16]))
	img, err := png.Decode(bytes.NewReader(b[16 : 8+n]))
	if err != nil || img.Bounds().Dx() != 128 {
		t.Fatalf("png: %v", err)
	}
	if _, _, _, a := img.At(64, 64).RGBA(); a == 0 {
		t.Fatal("icon centre is transparent")
	}
}

func TestSelfServiceLauncherQuoting(t *testing.T) {
	script := selfServiceLauncher(model.AgentSelfService{AppName: `O'Brien's "Apps"`})
	if !strings.Contains(script, `--title 'O'\''Brien'\''s "Apps"'`) {
		t.Fatalf("title not shell-quoted:\n%s", script)
	}
	if zsh, err := exec.LookPath("zsh"); err == nil {
		f := filepath.Join(t.TempDir(), "s")
		os.WriteFile(f, []byte(script), 0o755)
		if out, err := exec.Command(zsh, "-n", f).CombinedOutput(); err != nil {
			t.Fatalf("zsh syntax: %v %s", err, out)
		}
	}
}
