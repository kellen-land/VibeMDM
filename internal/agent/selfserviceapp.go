package agent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"howett.net/plist"

	"github.com/vibemdm/vibemdm/internal/model"
	"github.com/vibemdm/vibemdm/internal/selfservice"
)

// The Self Service app is a small bundle in /Applications whose executable opens a swiftDialog
// window showing the agent's local portal (http://127.0.0.1:8571). The agent creates it at
// enrollment and keeps it current at every check-in while Self Service is enabled, installing
// swiftDialog with Installomator when it is missing, and removes it when Self Service is turned off.

const (
	selfServiceBundleID = "com.vibemdm.selfservice"
	selfServiceMarker   = "vibemdm-selfservice" // file in Contents/Resources identifying our bundle
	selfServiceState    = SupportDir + "/selfservice-app.json"
	dialogBundleID      = "au.csiro.dialog"
)

// selfServiceApp keeps the app in line with the server's configuration.
type selfServiceApp struct {
	mu        sync.Mutex
	cfg       model.AgentSelfService
	busy      bool
	lastTry   time.Time
	appsDir   string // /Applications (tests use a temp dir)
	statePath string
	version   string
	ensureDlg func(ctx context.Context) error
	// compile builds the applet bundle at dest (osacompile); sign ad-hoc signs it (codesign).
	// macOS only; tests replace them.
	compile func(ctx context.Context, dest string) error
	sign    func(ctx context.Context, app string) error
	log     interface {
		Info(string, ...any)
		Warn(string, ...any)
	}
}

type selfServiceAppState struct {
	Path string `json:"path"`
	Hash string `json:"hash"`
}

// config returns the latest configuration (defaults before the first check-in).
func (s *selfServiceApp) config(org string) model.AgentSelfService {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.cfg
	if c.OrgName == "" {
		c.OrgName = org
	}
	c.AppName = selfservice.AppName(c.AppName, c.OrgName)
	if !selfservice.ValidAccent(c.Accent) {
		c.Accent = selfservice.DefaultAccent
	}
	return c
}

// Apply records a configuration from a check-in and reconciles the app in the background.
func (s *selfServiceApp) Apply(ctx context.Context, c model.AgentSelfService) {
	s.mu.Lock()
	s.cfg = c
	if s.busy {
		s.mu.Unlock()
		return
	}
	s.busy = true
	s.mu.Unlock()
	go func() {
		defer func() { s.mu.Lock(); s.busy = false; s.mu.Unlock() }()
		if err := s.reconcile(ctx); err != nil {
			s.log.Warn("could not set up the Self Service app", "component", "selfservice", "err", err)
		}
	}()
}

func (s *selfServiceApp) reconcile(ctx context.Context) error {
	c := s.config("")
	var st selfServiceAppState
	if b, err := os.ReadFile(s.statePath); err == nil {
		json.Unmarshal(b, &st)
	}
	path := filepath.Join(s.appsDir, c.AppName+".app")
	if !c.Enabled {
		if st.Path != "" {
			removeOurBundle(st.Path)
			os.Remove(s.statePath)
			s.log.Info("removed the Self Service app (Self Service is turned off)", "component", "selfservice", "path", st.Path)
		}
		return nil
	}
	// swiftDialog draws the window; without it the app falls back to the browser.
	if _, err := os.Stat(dialogBinary); err != nil && s.ensureDlg != nil {
		s.mu.Lock()
		due := time.Since(s.lastTry) > 6*time.Hour
		if due {
			s.lastTry = time.Now()
		}
		s.mu.Unlock()
		if due {
			if err := s.ensureDlg(ctx); err != nil {
				s.log.Warn("could not install swiftDialog for Self Service; the app opens in the browser until it is installed", "component", "selfservice", "err", err)
			} else {
				s.log.Info("installed swiftDialog for Self Service", "component", "selfservice")
			}
		}
	}
	files := selfServiceBundleFiles(c, s.version)
	hash := bundleHash(files)
	if st.Path == path && st.Hash == hash && bundleIntact(path) {
		return nil // up to date
	}
	if st.Path != "" && st.Path != path {
		removeOurBundle(st.Path) // renamed in the settings
	}
	if fi, err := os.Stat(path); err == nil && fi.IsDir() {
		if _, err := os.Stat(filepath.Join(path, "Contents", "Resources", selfServiceMarker)); err != nil {
			return fmt.Errorf("%s exists and was not created by VibeMDM; leaving it alone", path)
		}
	}
	// Build in the agent's support folder, then move into /Applications in one step.
	buildDir := filepath.Join(filepath.Dir(s.statePath), "selfservice-build")
	os.RemoveAll(buildDir)
	if err := os.MkdirAll(buildDir, 0o700); err != nil {
		return err
	}
	os.Chmod(buildDir, 0o700)
	defer os.RemoveAll(buildDir)
	tmp := filepath.Join(buildDir, c.AppName+".app")
	if err := s.buildBundle(ctx, tmp, c, files); err != nil {
		return err
	}
	os.RemoveAll(path)
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	// Refresh Finder's icon and name cache for the bundle.
	exec.Command("/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister", "-f", path).Run()
	b, _ := json.Marshal(selfServiceAppState{Path: path, Hash: hash})
	os.WriteFile(s.statePath, b, 0o600)
	s.log.Info("Self Service app installed", "component", "selfservice", "path", path)
	return nil
}

func removeOurBundle(path string) {
	if _, err := os.Stat(filepath.Join(path, "Contents", "Resources", selfServiceMarker)); err == nil {
		os.RemoveAll(path)
	}
}

type bundleFile struct {
	data []byte
	mode os.FileMode
}

// selfServiceBundleFiles returns the app bundle's files (paths relative to the .app).
func selfServiceBundleFiles(c model.AgentSelfService, version string) map[string]bundleFile {
	return map[string]bundleFile{
		"Contents/Resources/launcher.zsh":         {[]byte(selfServiceLauncher(c)), 0o755},
		"Contents/Resources/applet.icns":          {selfServiceIcon(c.Accent), 0o644},
		"Contents/Resources/" + selfServiceMarker: {[]byte("Created by the VibeMDM agent; it is replaced at each check-in.\n" + c.AppName + "\n" + version + "\n"), 0o644},
	}
}

func bundleHash(files map[string]bundleFile) string {
	h := sha256.New()
	for _, rel := range []string{"Contents/Resources/launcher.zsh", "Contents/Resources/applet.icns", "Contents/Resources/" + selfServiceMarker} {
		h.Write([]byte(rel))
		h.Write(files[rel].data)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// shellQuote quotes s for zsh single quotes.
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// selfServiceLauncher is run by the applet (Contents/Resources/launcher.zsh): one swiftDialog
// window per user showing the local portal; the browser when swiftDialog is not installed.
func selfServiceLauncher(c model.AgentSelfService) string {
	url := "http://" + SelfServicePort + "/?embedded=1"
	return `#!/bin/zsh
# ` + c.AppName + ` — created by the VibeMDM agent; it is replaced at each check-in.
DIALOG=` + dialogBinary + `
URL=` + shellQuote(url) + `
APP="${0:A:h:h:h}"
if [[ ! -x "$DIALOG" ]]; then
  exec /usr/bin/open "$URL"
fi
CMD="${TMPDIR:-/tmp}/vibemdm-selfservice-$UID.cmd"
# One window per user: bring an open one to the front instead of opening another.
if /usr/bin/pgrep -u "$UID" -f -- "--commandfile $CMD" >/dev/null 2>&1; then
  /usr/bin/osascript -e 'tell application id "` + dialogBundleID + `" to activate' >/dev/null 2>&1
  exit 0
fi
: > "$CMD"
exec "$DIALOG" \
  --title ` + shellQuote(c.AppName) + ` --titlefont 'size=20' \
  --message ' ' --icon "$APP" --iconsize 48 \
  --webcontent "$URL" \
  --width 1100 --height 760 --resizable --moveable \
  --button1text 'Done' \
  --commandfile "$CMD"
`
}

// ---- icon ----

func parseAccent(hexc string) color.RGBA {
	if !selfservice.ValidAccent(hexc) {
		hexc = selfservice.DefaultAccent
	}
	v, _ := strconv.ParseUint(hexc[1:], 16, 32)
	return color.RGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 255}
}

// iconCyan is the colour the icon fades to (the console's cyan).
var iconCyan = color.RGBA{0x22, 0xd3, 0xee, 255}

// drawIcon draws the app icon at size×size: a rounded square fading from the accent colour to
// cyan, and a white shopping bag.
func drawIcon(size int, accent color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	s := float64(size)
	inset, radius := s*0.1, s*0.18
	lo, hi := inset, s-inset
	inRounded := func(x, y float64) float64 { // coverage with a 1px soft edge
		cx := math.Max(lo+radius, math.Min(x, hi-radius))
		cy := math.Max(lo+radius, math.Min(y, hi-radius))
		d := math.Hypot(x-cx, y-cy) - radius
		if x < lo || x > hi || y < lo || y > hi {
			d = math.Max(d, 1)
		}
		return math.Max(0, math.Min(1, 0.5-d))
	}
	white := func(x, y float64) float64 {
		// Bag body: trapezoid-ish rounded rectangle.
		bx0, bx1, by0, by1 := s*0.31, s*0.69, s*0.40, s*0.74
		in := 0.0
		if x >= bx0 && x <= bx1 && y >= by0 && y <= by1 {
			in = 1
		}
		// Handle: ring above the body.
		hcx, hcy, r := s*0.5, s*0.40, s*0.10
		d := math.Abs(math.Hypot(x-hcx, y-hcy) - r)
		if y <= hcy && d <= s*0.022 {
			in = 1
		}
		return in
	}
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			fx, fy := float64(x)+0.5, float64(y)+0.5
			a := inRounded(fx, fy)
			if a == 0 {
				continue
			}
			// Diagonal fade from the accent (top left) towards the console's cyan (bottom
			// right), with the top slightly lifted.
			t := (fy - lo) / (hi - lo)
			diag := math.Max(0, math.Min(1, ((fx-lo)+(fy-lo))/(2*(hi-lo))))
			mix := 0.5 * diag * diag
			r := float64(accent.R) + (float64(iconCyan.R)-float64(accent.R))*mix
			g := float64(accent.G) + (float64(iconCyan.G)-float64(accent.G))*mix
			b := float64(accent.B) + (float64(iconCyan.B)-float64(accent.B))*mix
			lift := 0.14 * (1 - t)
			r, g, b = r+(255-r)*lift, g+(255-g)*lift, b+(255-b)*lift
			if w := white(fx, fy); w > 0 {
				r, g, b = 255, 255, 255
			}
			img.SetRGBA(x, y, color.RGBA{uint8(r * a), uint8(g * a), uint8(b * a), uint8(255 * a)})
		}
	}
	return img
}

// selfServiceIcon builds an .icns with PNG images at 128, 256, 512 and 1024 pixels.
func selfServiceIcon(accentHex string) []byte {
	accent := parseAccent(accentHex)
	var body bytes.Buffer
	for _, e := range []struct {
		typ  string
		size int
	}{{"ic07", 128}, {"ic08", 256}, {"ic09", 512}, {"ic10", 1024}} {
		var pngb bytes.Buffer
		png.Encode(&pngb, drawIcon(e.size, accent))
		body.WriteString(e.typ)
		binary.Write(&body, binary.BigEndian, uint32(8+pngb.Len()))
		body.Write(pngb.Bytes())
	}
	var out bytes.Buffer
	out.WriteString("icns")
	binary.Write(&out, binary.BigEndian, uint32(8+body.Len()))
	out.Write(body.Bytes())
	return out.Bytes()
}

// bundleIntact reports whether an installed bundle is ours and usable: the marker exists, the
// folders are readable by everyone and the applet is executable.
func bundleIntact(path string) bool {
	if _, err := os.Stat(filepath.Join(path, "Contents", "Resources", selfServiceMarker)); err != nil {
		return false
	}
	for _, d := range []string{path, filepath.Join(path, "Contents"), filepath.Join(path, "Contents", "MacOS"), filepath.Join(path, "Contents", "Resources")} {
		if fi, err := os.Stat(d); err != nil || fi.Mode().Perm()&0o555 != 0o555 {
			return false
		}
	}
	for _, f := range []string{"Contents/MacOS/applet", "Contents/Resources/launcher.zsh"} {
		if fi, err := os.Stat(filepath.Join(path, f)); err != nil || fi.Mode().Perm()&0o555 != 0o555 {
			return false
		}
	}
	return true
}

// buildBundle makes the app at dest: an AppleScript applet (a real Mach-O executable, which
// macOS requires; a script as the main executable is refused) that runs launcher.zsh in the
// background, with our icon, name and bundle identifier, explicit modes and an ad-hoc signature.
func (s *selfServiceApp) buildBundle(ctx context.Context, dest string, c model.AgentSelfService, files map[string]bundleFile) error {
	compile, sign := s.compile, s.sign
	if compile == nil {
		compile = osacompileApplet
	}
	if sign == nil {
		sign = codesignAdHoc
	}
	if err := compile(ctx, dest); err != nil {
		return fmt.Errorf("building the applet: %w", err)
	}
	res := filepath.Join(dest, "Contents", "Resources")
	os.RemoveAll(filepath.Join(res, "Assets.car")) // its icon would win over ours
	for rel, f := range files {
		p := filepath.Join(dest, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, f.data, f.mode); err != nil {
			return err
		}
	}
	if err := editInfoPlist(filepath.Join(dest, "Contents", "Info.plist"), c, s.version); err != nil {
		return err
	}
	// Modes are set explicitly: the process umask must not decide whether users can open the app.
	if err := filepath.Walk(dest, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		mode := os.FileMode(0o644)
		rel, _ := filepath.Rel(dest, p)
		rel = filepath.ToSlash(rel)
		switch {
		case fi.IsDir(), rel == "Contents/MacOS/applet", rel == "Contents/Resources/launcher.zsh":
			mode = 0o755
		}
		return os.Chmod(p, mode)
	}); err != nil {
		return err
	}
	if err := sign(ctx, dest); err != nil {
		return fmt.Errorf("signing the app: %w", err)
	}
	return nil
}

// appletSource runs the launcher next to the applet and returns at once.
const appletSource = `do shell script "/bin/zsh " & quoted form of (POSIX path of (path to me) & "Contents/Resources/launcher.zsh") & " >/dev/null 2>&1 &"`

func osacompileApplet(ctx context.Context, dest string) error {
	out, err := exec.CommandContext(ctx, "/usr/bin/osacompile", "-o", dest, "-e", appletSource).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func codesignAdHoc(ctx context.Context, app string) error {
	out, err := exec.CommandContext(ctx, "/usr/bin/codesign", "--force", "--deep", "--sign", "-", app).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// editInfoPlist gives the applet our identity: bundle identifier, name, version, icon, and no
// Dock icon of its own (the swiftDialog window has one).
func editInfoPlist(path string, c model.AgentSelfService, version string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var info map[string]any
	if _, err := plist.Unmarshal(raw, &info); err != nil {
		return fmt.Errorf("reading the applet's Info.plist: %w", err)
	}
	info["CFBundleIdentifier"] = selfServiceBundleID
	info["CFBundleName"] = c.AppName
	info["CFBundleDisplayName"] = c.AppName
	info["CFBundleShortVersionString"] = "1.0"
	info["CFBundleVersion"] = strings.TrimPrefix(version, "v")
	info["CFBundleIconFile"] = "applet"
	delete(info, "CFBundleIconName")
	info["LSUIElement"] = true
	info["LSMinimumSystemVersion"] = "12.0"
	out, err := plist.MarshalIndent(info, plist.XMLFormat, "\t")
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o644)
}
