package agent

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"html/template"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/vibemdm/vibemdm/internal/model"
	"github.com/vibemdm/vibemdm/internal/selfservice"
)

// SelfServicePort is where the local Self Service portal listens (loopback only). The Self
// Service app shows it in a swiftDialog window; it also works in any browser on the Mac.
const SelfServicePort = "127.0.0.1:8571"

//go:embed selfservice.html
var ssHTML string

var ssPage = template.Must(template.New("ss").Parse(ssHTML))

// ssConfig is injected into the page as JSON.
type ssConfig struct {
	AppName      string `json:"appName"`
	Org          string `json:"org"`
	Welcome      string `json:"welcome"`
	WelcomeTitle string `json:"welcomeTitle"`
	Embedded     bool   `json:"embedded"`
}

// ssJob is one Self Service run (also kept as history).
type ssJob struct {
	ID       string    `json:"id"`
	Key      string    `json:"key"` // kind:id of the item
	Name     string    `json:"name"`
	Status   string    `json:"status"` // queued | running | completed | failed
	Message  string    `json:"message"`
	Started  time.Time `json:"started"`
	Finished time.Time `json:"finished,omitempty"`
}

// ssJobs tracks runs started from the portal and keeps the last 50 as history on disk.
type ssJobs struct {
	mu   sync.Mutex
	list []*ssJob
	path string
}

func newSSJobs(path string) *ssJobs {
	j := &ssJobs{path: path}
	if b, err := os.ReadFile(path); err == nil {
		json.Unmarshal(b, &j.list)
	}
	for _, x := range j.list { // a restart interrupted these
		if x.Status == "queued" || x.Status == "running" {
			x.Status, x.Message = "failed", "interrupted (the agent restarted)"
		}
	}
	return j
}

func (j *ssJobs) add(key, name string) *ssJob {
	j.mu.Lock()
	defer j.mu.Unlock()
	x := &ssJob{ID: uuid.NewString(), Key: key, Name: name, Status: "queued", Started: time.Now().UTC()}
	j.list = append([]*ssJob{x}, j.list...)
	if len(j.list) > 50 {
		j.list = j.list[:50]
	}
	return x
}

func (j *ssJobs) update(x *ssJob, status, msg string) {
	j.mu.Lock()
	x.Status, x.Message = status, msg
	if status == "completed" || status == "failed" {
		x.Finished = time.Now().UTC()
	}
	b, _ := json.Marshal(j.list)
	j.mu.Unlock()
	if j.path != "" {
		os.WriteFile(j.path, b, 0o600)
	}
}

func (j *ssJobs) busy(key string) bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	for _, x := range j.list {
		if x.Key == key && (x.Status == "queued" || x.Status == "running") {
			return true
		}
	}
	return false
}

// snapshot returns copies of the jobs (latest per item when latestOnly).
func (j *ssJobs) snapshot(latestOnly bool) []ssJob {
	j.mu.Lock()
	defer j.mu.Unlock()
	out := []ssJob{}
	seen := map[string]bool{}
	for _, x := range j.list {
		if latestOnly {
			if seen[x.Key] {
				continue
			}
			seen[x.Key] = true
		}
		out = append(out, *x)
	}
	return out
}

// selfServiceGuard protects the loopback portal from other websites: the Host
// must be the portal itself (defeats DNS rebinding), and state-changing requests
// need a custom header and JSON body, which cross-site pages cannot send without
// a CORS preflight that this server never approves.
func selfServiceGuard(next http.Handler) http.Handler {
	_, port, _ := net.SplitHostPort(SelfServicePort)
	allowed := map[string]bool{SelfServicePort: true, "localhost:" + port: true, "127.0.0.1:" + port: true}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !allowed[r.Host] {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if r.Header.Get("X-VibeMDM-SelfService") != "1" || !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			if o := r.Header.Get("Origin"); o != "" && o != "http://"+r.Host {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
		}
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}

// appNameRe limits icon lookups to plain application names.
var appNameRe = regexp.MustCompile(`^[^/\\:\x00]{1,100}$`)

// installedApp returns the path of an installed app by name ("" if not found).
func installedApp(name string) string {
	if !appNameRe.MatchString(name) || strings.HasPrefix(name, ".") {
		return ""
	}
	for _, dir := range []string{"/Applications", "/Applications/Utilities", "/System/Applications"} {
		p := filepath.Join(dir, name+".app")
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			return p
		}
	}
	return ""
}

// appIconPNG renders an installed app's icon as a 128-pixel PNG, cached by app path and mtime.
func appIconPNG(ctx context.Context, app string) ([]byte, error) {
	fi, err := os.Stat(app)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256([]byte(app + fi.ModTime().String()))
	cache := filepath.Join(SupportDir, "cache", "icons", hex.EncodeToString(sum[:8])+".png")
	if b, err := os.ReadFile(cache); err == nil {
		return b, nil
	}
	os.MkdirAll(filepath.Dir(cache), 0o755)
	icon := ""
	if out, err := exec.CommandContext(ctx, "/usr/bin/defaults", "read", filepath.Join(app, "Contents", "Info"), "CFBundleIconFile").Output(); err == nil {
		name := strings.TrimSpace(string(out))
		if !strings.HasSuffix(name, ".icns") {
			name += ".icns"
		}
		if p := filepath.Join(app, "Contents", "Resources", filepath.Base(name)); fileExists(p) {
			icon = p
		}
	}
	if icon == "" {
		return nil, os.ErrNotExist // asset-catalog-only icons: the page shows a letter tile
	}
	if err := exec.CommandContext(ctx, "/usr/bin/sips", "-s", "format", "png", "--resampleHeightWidthMax", "128", icon, "--out", cache).Run(); err != nil {
		return nil, err
	}
	return os.ReadFile(cache)
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

// serveSelfService hosts the local Self Service portal (loopback only).
func (d *Daemon) serveSelfService(ctx context.Context) {
	if d.ssJobs == nil {
		d.ssJobs = newSSJobs(filepath.Join(SupportDir, "selfservice-history.json"))
	}
	jsonOut := func(w http.ResponseWriter, code int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(code)
		json.NewEncoder(w).Encode(v)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		c := d.selfServiceConfig()
		welcome := c.Welcome
		if welcome == "" {
			welcome = "Apps and tools your IT team has made available for this Mac. Choose one to install it, no administrator password needed."
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		ssPage.Execute(w, map[string]any{"AppName": c.AppName, "Accent": template.CSS(c.Accent),
			"ConfigJSON": ssConfig{AppName: c.AppName, Org: c.OrgName, Welcome: welcome, WelcomeTitle: "Welcome to " + c.AppName, Embedded: r.URL.Query().Get("embedded") == "1"}})
	})
	mux.HandleFunc("GET /api/items", func(w http.ResponseWriter, r *http.Request) {
		items, _, err := d.client.SelfServiceList(r.Context())
		if err != nil {
			jsonOut(w, 502, map[string]string{"error": "cannot reach the management server"})
			return
		}
		type item struct {
			model.SelfServiceItem
			Installed bool `json:"installed"`
		}
		out := make([]item, 0, len(items))
		for _, it := range items {
			out = append(out, item{SelfServiceItem: it, Installed: it.AppName != "" && installedApp(it.AppName) != ""})
		}
		sort.SliceStable(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
		jsonOut(w, 200, map[string]any{"items": out})
	})
	mux.HandleFunc("GET /api/icon", func(w http.ResponseWriter, r *http.Request) {
		app := installedApp(r.URL.Query().Get("app"))
		if app == "" {
			http.NotFound(w, r)
			return
		}
		b, err := appIconPNG(r.Context(), app)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "max-age=3600")
		w.Write(b)
	})
	mux.HandleFunc("GET /api/jobs", func(w http.ResponseWriter, r *http.Request) {
		jsonOut(w, 200, map[string]any{"items": d.ssJobs.snapshot(true)})
	})
	mux.HandleFunc("GET /api/history", func(w http.ResponseWriter, r *http.Request) {
		jsonOut(w, 200, map[string]any{"items": d.ssJobs.snapshot(false)})
	})
	mux.HandleFunc("GET /api/device", func(w http.ResponseWriter, r *http.Request) {
		si := CollectSystemInfo(r.Context())
		fv := run(r.Context(), 5*time.Second, "fdesetup", "status")
		switch {
		case strings.Contains(fv, "FileVault is On"):
			fv = "On"
		case strings.Contains(fv, "FileVault is Off"):
			fv = "Off"
		}
		managedBy := d.cfg.ServerURL
		if u := strings.TrimPrefix(strings.TrimPrefix(managedBy, "https://"), "http://"); u != "" {
			managedBy = d.selfServiceConfig().OrgName + " (" + strings.SplitN(u, "/", 2)[0] + ")"
		}
		last := ""
		if t := d.exec.LastCheckin(); !t.IsZero() {
			last = t.Format(time.RFC3339)
		}
		jsonOut(w, 200, map[string]string{"computer_name": si.ComputerName, "model": firstNonEmpty(si.ModelName, si.ModelIdentifier), "serial_number": si.SerialNumber,
			"os_version": strings.TrimSpace(si.OSVersion + " (" + si.OSBuild + ")"), "filevault": fv, "ip_address": si.IP, "last_checkin": last,
			"managed_by": managedBy, "agent_version": d.version})
	})
	mux.HandleFunc("POST /api/checkin", func(w http.ResponseWriter, r *http.Request) {
		select {
		case d.checkinNow <- "checkin":
			jsonOut(w, 200, map[string]bool{"ok": true})
		default:
			jsonOut(w, 409, map[string]string{"error": "a check-in is already in progress"})
		}
	})
	mux.HandleFunc("POST /api/run", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Kind, ID string }
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
			jsonOut(w, 400, map[string]string{"error": "invalid request"})
			return
		}
		key := req.Kind + ":" + req.ID
		if d.ssJobs.busy(key) {
			jsonOut(w, 409, map[string]string{"error": "this item is already running"})
			return
		}
		pol, app, err := d.client.SelfServiceRun(r.Context(), req.Kind, req.ID)
		if err != nil {
			jsonOut(w, 502, map[string]string{"error": err.Error()})
			return
		}
		name := ""
		switch {
		case pol != nil:
			name = pol.Name
		case app != nil:
			name = app.Name
		}
		job := d.ssJobs.add(key, name)
		go d.runLocked(ctx, func(ctx context.Context) {
			d.ssJobs.update(job, "running", "")
			switch {
			case pol != nil:
				sub := d.exec.RunPolicy(ctx, *pol)
				msg := ""
				for _, st := range sub.Steps {
					if st.Status == "failed" {
						msg = st.Name + ": " + Summarize(st.Output, 1, 160)
						break
					}
				}
				d.ssJobs.update(job, map[bool]string{true: "completed", false: "failed"}[sub.Status == "completed"], msg)
			case app != nil:
				if d.exec.InstallApp(ctx, *app) {
					d.ssJobs.update(job, "completed", "")
				} else {
					d.ssJobs.update(job, "failed", "the installation failed; IT can see the details")
				}
			}
		})
		jsonOut(w, 202, map[string]any{"ok": true, "job": job.ID})
	})
	srv := &http.Server{Addr: SelfServicePort, Handler: selfServiceGuard(mux), ReadHeaderTimeout: 10 * time.Second}
	ln, err := net.Listen("tcp", SelfServicePort)
	if err != nil {
		d.log.Warn("self service portal not started", "component", "selfservice", "err", err)
		return
	}
	go func() {
		<-ctx.Done()
		srv.Close()
	}()
	d.log.Info("self service portal available", "component", "selfservice", "url", "http://"+SelfServicePort)
	srv.Serve(ln)
}

// selfServiceConfig is the current Self Service configuration (defaults before the first check-in).
func (d *Daemon) selfServiceConfig() model.AgentSelfService {
	if d.ssApp == nil {
		return model.AgentSelfService{Enabled: true, OrgName: d.cfg.OrgName, AppName: selfservice.AppName("", d.cfg.OrgName), Accent: selfservice.DefaultAccent}
	}
	return d.ssApp.config(d.cfg.OrgName)
}
