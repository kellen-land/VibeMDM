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
	"syscall"
	"testing"

	"github.com/vibemdm/vibemdm/internal/model"
)

// newTestSSApp replaces osacompile and codesign with a fake applet like the one osacompile makes.
func newTestSSApp(t *testing.T) *selfServiceApp {
	dir := t.TempDir()
	return &selfServiceApp{appsDir: filepath.Join(dir, "Applications"), statePath: filepath.Join(dir, "state.json"), version: "v1", log: nopLog{},
		compile: func(_ context.Context, dest string) error {
			// Owner-only, as osacompile leaves it under the agent's old 0177 umask (root can still
			// write there; the test user needs the explicit chmod).
			for _, d := range []string{dest, filepath.Join(dest, "Contents"), filepath.Join(dest, "Contents", "MacOS"), filepath.Join(dest, "Contents", "Resources")} {
				os.Mkdir(d, 0o700)
				os.Chmod(d, 0o700)
			}
			os.WriteFile(filepath.Join(dest, "Contents", "MacOS", "applet"), []byte("\xcf\xfa\xed\xfe"), 0o600)
			os.WriteFile(filepath.Join(dest, "Contents", "Resources", "Assets.car"), []byte("x"), 0o600)
			return os.WriteFile(filepath.Join(dest, "Contents", "Info.plist"), []byte(`<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict><key>CFBundleExecutable</key><string>applet</string><key>CFBundleIconName</key><string>applet</string><key>CFBundleIdentifier</key><string>com.apple.ScriptEditor.id.x</string></dict></plist>`), 0o600)
		},
		sign: func(context.Context, string) error { return nil },
	}
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
	exe := filepath.Join(app, "Contents", "Resources", "launcher.zsh")
	for _, f := range []string{exe, filepath.Join(app, "Contents", "MacOS", "applet")} {
		fi, err := os.Stat(f)
		if err != nil || fi.Mode().Perm() != 0o755 {
			t.Fatalf("%s missing or not executable: %v", f, err)
		}
	}
	plist, _ := os.ReadFile(filepath.Join(app, "Contents", "Info.plist"))
	for _, want := range []string{"<string>Lands School Self Service</string>", selfServiceBundleID, "<key>LSUIElement</key>", "<key>CFBundleExecutable</key>"} {
		if !strings.Contains(string(plist), want) {
			t.Fatalf("Info.plist lacks %q: %s", want, plist)
		}
	}
	if strings.Contains(string(plist), "CFBundleIconName") || fileExists(filepath.Join(app, "Contents", "Resources", "Assets.car")) {
		t.Fatal("the applet's own icon would override ours")
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

func TestSelfServiceAppModesIgnoreUmask(t *testing.T) {
	s := newTestSSApp(t)
	os.MkdirAll(s.appsDir, 0o755)
	old := syscall.Umask(0o177) // what the agent's socket setup used to leave behind
	defer syscall.Umask(old)
	s.cfg = model.AgentSelfService{Enabled: true, OrgName: "Lands"}
	if err := s.reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	app := filepath.Join(s.appsDir, "Lands Self Service.app")
	for rel, want := range map[string]os.FileMode{".": 0o755, "Contents": 0o755, "Contents/MacOS": 0o755, "Contents/MacOS/applet": 0o755,
		"Contents/Resources/launcher.zsh": 0o755, "Contents/Info.plist": 0o644, "Contents/Resources/applet.icns": 0o644} {
		fi, err := os.Stat(filepath.Join(app, rel))
		if err != nil || fi.Mode().Perm() != want {
			t.Errorf("%s: %v %v, want %v", rel, fi.Mode().Perm(), err, want)
		}
	}
	// A bundle whose modes were damaged is repaired even though its contents are current.
	os.Chmod(filepath.Join(app, "Contents", "MacOS", "applet"), 0o600)
	if err := s.reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(filepath.Join(app, "Contents", "MacOS", "applet")); fi.Mode().Perm() != 0o755 {
		t.Fatal("damaged launcher not repaired")
	}
}
