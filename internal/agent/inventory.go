package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"howett.net/plist"

	"github.com/vibemdm/vibemdm/internal/model"
)

// run executes a command with a timeout and returns trimmed stdout.
func run(ctx context.Context, timeout time.Duration, name string, args ...string) string {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	out, _ := exec.CommandContext(ctx, name, args...).Output()
	return strings.TrimSpace(string(out))
}

func runCombined(ctx context.Context, timeout time.Duration, name string, args ...string) string {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	out, _ := exec.CommandContext(ctx, name, args...).CombinedOutput()
	return strings.TrimSpace(string(out))
}

// SystemInfo is the quick subset of inventory sent with every check-in.
type SystemInfo struct {
	Hostname, ComputerName, LocalHostname, SerialNumber, UDID, ModelIdentifier, ModelName, OSVersion, OSBuild, Arch, IP, MAC, ConsoleUser string
	Uptime                                                                                                                                time.Duration
}

// CollectSystemInfo gathers basic identifiers quickly.
func CollectSystemInfo(ctx context.Context) SystemInfo {
	si := SystemInfo{Arch: runtime.GOARCH}
	si.Hostname, _ = os.Hostname()
	si.ComputerName = run(ctx, 5*time.Second, "scutil", "--get", "ComputerName")
	si.LocalHostname = run(ctx, 5*time.Second, "scutil", "--get", "LocalHostName")
	si.OSVersion = run(ctx, 5*time.Second, "sw_vers", "-productVersion")
	si.OSBuild = run(ctx, 5*time.Second, "sw_vers", "-buildVersion")
	si.ModelIdentifier = run(ctx, 5*time.Second, "sysctl", "-n", "hw.model")
	if m := run(ctx, 5*time.Second, "uname", "-m"); m != "" {
		si.Arch = m
	}
	// Serial & hardware UUID from IORegistry (fast, no system_profiler).
	ioreg := run(ctx, 10*time.Second, "ioreg", "-rd1", "-c", "IOPlatformExpertDevice")
	si.SerialNumber = ioregValue(ioreg, "IOPlatformSerialNumber")
	si.UDID = ioregValue(ioreg, "IOPlatformUUID")
	si.ConsoleUser = ConsoleUser(ctx)
	if iface := primaryInterface(ctx); iface != "" {
		si.IP = run(ctx, 5*time.Second, "ipconfig", "getifaddr", iface)
		ifc := run(ctx, 5*time.Second, "ifconfig", iface)
		if m := regexp.MustCompile(`ether ([0-9a-f:]{17})`).FindStringSubmatch(ifc); len(m) == 2 {
			si.MAC = m[1]
		}
	}
	si.Uptime = Uptime(ctx)
	return si
}

func ioregValue(ioreg, key string) string {
	re := regexp.MustCompile(`"` + key + `" = "([^"]+)"`)
	if m := re.FindStringSubmatch(ioreg); len(m) == 2 {
		return m[1]
	}
	return ""
}

func primaryInterface(ctx context.Context) string {
	out := run(ctx, 5*time.Second, "route", "-n", "get", "default")
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "interface:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "interface:"))
		}
	}
	return ""
}

// ConsoleUser returns the user logged in at the console ("" if none).
func ConsoleUser(ctx context.Context) string {
	u := run(ctx, 5*time.Second, "stat", "-f", "%Su", "/dev/console")
	if u == "root" || u == "_mbsetupuser" || u == "loginwindow" {
		return ""
	}
	return u
}

// ConsoleUserUID returns the uid of the console user.
func ConsoleUserUID(ctx context.Context, user string) int {
	if user == "" {
		return 0
	}
	n, _ := strconv.Atoi(run(ctx, 5*time.Second, "id", "-u", user))
	return n
}

// Uptime returns system uptime.
func Uptime(ctx context.Context) time.Duration {
	out := run(ctx, 5*time.Second, "sysctl", "-n", "kern.boottime")
	// { sec = 1700000000, usec = 0 } Tue Nov ...
	if m := regexp.MustCompile(`sec = (\d+)`).FindStringSubmatch(out); len(m) == 2 {
		if sec, err := strconv.ParseInt(m[1], 10, 64); err == nil {
			return time.Since(time.Unix(sec, 0))
		}
	}
	return 0
}

// CollectInventory gathers the full inventory. Independent collectors run
// concurrently so a full recon typically completes in a few seconds.
func CollectInventory(ctx context.Context, version string, eas []model.ExtensionAttribute, runEA func(ctx context.Context, ea model.ExtensionAttribute) string) model.AgentInventory {
	started := time.Now()
	si := CollectSystemInfo(ctx)
	inv := model.AgentInventory{
		AgentVersion: version, ComputerName: si.ComputerName, Hostname: si.Hostname, LocalHostname: si.LocalHostname,
		SerialNumber: si.SerialNumber, UDID: si.UDID, ModelIdentifier: si.ModelIdentifier, OSVersion: si.OSVersion, OSBuild: si.OSBuild,
		Architecture: si.Arch, IPAddress: si.IP, MACAddress: si.MAC, ConsoleUser: si.ConsoleUser, UptimeSeconds: int64(si.Uptime.Seconds()),
		Hardware: map[string]any{}, Extra: map[string]any{}, ExtensionAttributes: map[string]string{}, CollectedAt: time.Now().UTC(),
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	par := func(fn func()) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fn()
		}()
	}
	par(func() {
		// Hardware via system_profiler (JSON).
		out := run(ctx, 60*time.Second, "system_profiler", "SPHardwareDataType", "-json")
		var doc struct {
			SPHardwareDataType []map[string]any `json:"SPHardwareDataType"`
		}
		mu.Lock()
		defer mu.Unlock()
		if out != "" && json.Unmarshal([]byte(out), &doc) == nil && len(doc.SPHardwareDataType) > 0 {
			hw := doc.SPHardwareDataType[0]
			inv.Hardware = hw
			inv.ModelName = str(hw["machine_name"])
			if inv.SerialNumber == "" {
				inv.SerialNumber = str(hw["serial_number"])
			}
			if inv.UDID == "" {
				inv.UDID = str(hw["platform_UUID"])
			}
			inv.CPU = firstNonEmpty(str(hw["chip_type"]), str(hw["cpu_type"]))
			if n := str(hw["number_processors"]); n != "" {
				n = strings.TrimPrefix(n, "proc ")
				if i := strings.Index(n, ":"); i > 0 {
					n = n[:i]
				}
				inv.CPUCores, _ = strconv.Atoi(strings.TrimSpace(n))
			}
		}
		if inv.CPU == "" {
			inv.CPU = run(ctx, 5*time.Second, "sysctl", "-n", "machdep.cpu.brand_string")
		}
		if inv.CPUCores == 0 {
			inv.CPUCores, _ = strconv.Atoi(run(ctx, 5*time.Second, "sysctl", "-n", "hw.ncpu"))
		}
	})
	par(func() {
		mem, _ := strconv.ParseInt(run(ctx, 5*time.Second, "sysctl", "-n", "hw.memsize"), 10, 64)
		var total, avail int64
		if out := run(ctx, 10*time.Second, "df", "-k", "/"); out != "" {
			lines := strings.Split(out, "\n")
			if len(lines) >= 2 {
				f := strings.Fields(lines[len(lines)-1])
				if len(f) >= 4 {
					total, _ = strconv.ParseInt(f[1], 10, 64)
					avail, _ = strconv.ParseInt(f[3], 10, 64)
				}
			}
		}
		mu.Lock()
		inv.MemoryMB = mem / 1024 / 1024
		inv.DiskTotalMB, inv.DiskFreeMB = total/1024, avail/1024
		mu.Unlock()
	})
	par(func() {
		full := ""
		if si.ConsoleUser != "" {
			full = dsclAttr(ctx, si.ConsoleUser, "RealName")
		}
		last := firstNonEmpty(si.ConsoleUser, run(ctx, 5*time.Second, "defaults", "read", "/Library/Preferences/com.apple.loginwindow", "lastUserName"))
		mu.Lock()
		inv.ConsoleUserFullName, inv.LastUser = full, last
		mu.Unlock()
	})
	par(func() {
		fv := boolp(strings.Contains(run(ctx, 15*time.Second, "fdesetup", "status"), "FileVault is On"))
		var sip, fw, gk *bool
		if s := run(ctx, 5*time.Second, "csrutil", "status"); s != "" {
			sip = boolp(strings.Contains(strings.ToLower(s), "enabled"))
		}
		if s := run(ctx, 5*time.Second, "/usr/libexec/ApplicationFirewall/socketfilterfw", "--getglobalstate"); s != "" {
			fw = boolp(strings.Contains(strings.ToLower(s), "enabled"))
		}
		if s := runCombined(ctx, 5*time.Second, "spctl", "--status"); s != "" {
			gk = boolp(strings.Contains(s, "assessments enabled"))
		}
		mu.Lock()
		inv.FileVaultEnabled, inv.SIPEnabled, inv.FirewallEnabled, inv.GatekeeperEnabled = fv, sip, fw, gk
		mu.Unlock()
	})
	par(func() {
		out := run(ctx, 30*time.Second, "system_profiler", "SPiBridgeDataType", "-json")
		var doc struct {
			SPiBridgeDataType []map[string]any `json:"SPiBridgeDataType"`
		}
		if out != "" && json.Unmarshal([]byte(out), &doc) == nil && len(doc.SPiBridgeDataType) > 0 {
			mu.Lock()
			inv.SecureBootLevel = str(doc.SPiBridgeDataType[0]["ibridge_secure_boot"])
			inv.Extra["ibridge"] = doc.SPiBridgeDataType[0]
			mu.Unlock()
		}
	})
	par(func() {
		if batt := run(ctx, 5*time.Second, "pmset", "-g", "batt"); strings.Contains(batt, "%") {
			if m := regexp.MustCompile(`(\d+)%`).FindStringSubmatch(batt); len(m) == 2 {
				if n, err := strconv.Atoi(m[1]); err == nil {
					mu.Lock()
					inv.BatteryPercent = &n
					mu.Unlock()
				}
			}
		}
	})
	par(func() {
		apps := CollectApps(ctx)
		mu.Lock()
		inv.Apps = apps
		mu.Unlock()
	})
	par(func() {
		profiles := CollectProfiles(ctx)
		mu.Lock()
		inv.Profiles = profiles
		mu.Unlock()
	})
	par(func() {
		users := CollectLocalUsers(ctx)
		mu.Lock()
		inv.LocalUsers = users
		mu.Unlock()
	})
	// Extension attribute scripts: bounded concurrency.
	if runEA != nil && len(eas) > 0 {
		sem := make(chan struct{}, 4)
		for _, ea := range eas {
			ea := ea
			par(func() {
				sem <- struct{}{}
				defer func() { <-sem }()
				v := runEA(ctx, ea)
				mu.Lock()
				inv.ExtensionAttributes[ea.ID] = v
				mu.Unlock()
			})
		}
	}
	wg.Wait()
	inv.CollectDurationMS = time.Since(started).Milliseconds()
	return inv
}

func dsclAttr(ctx context.Context, user, attr string) string {
	out := run(ctx, 5*time.Second, "dscl", ".", "-read", "/Users/"+user, attr)
	// "RealName:\n Jane Doe" or "RealName: Jane Doe"
	out = strings.TrimPrefix(out, attr+":")
	return strings.TrimSpace(out)
}

func boolp(b bool) *bool { return &b }

func str(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case nil:
		return ""
	default:
		return fmt.Sprint(t)
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// CollectApps walks the application folders and reads Info.plist files.
func CollectApps(ctx context.Context) []model.ComputerApp {
	roots := []string{"/Applications", "/System/Applications", "/Library/Application Support/VibeMDM/.."}
	roots = roots[:2]
	if homes, _ := filepath.Glob("/Users/*/Applications"); len(homes) > 0 {
		roots = append(roots, homes...)
	}
	seen := map[string]bool{}
	var apps []model.ComputerApp
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			if ctx.Err() != nil {
				return
			}
			p := filepath.Join(dir, e.Name())
			if strings.HasSuffix(e.Name(), ".app") {
				if seen[p] {
					continue
				}
				seen[p] = true
				if a, ok := readAppBundle(p); ok {
					apps = append(apps, a)
				}
				continue
			}
			if e.IsDir() && depth < 2 && !strings.HasPrefix(e.Name(), ".") {
				walk(p, depth+1)
			}
		}
	}
	for _, r := range roots {
		walk(r, 0)
	}
	return apps
}

func readAppBundle(path string) (model.ComputerApp, bool) {
	b, err := os.ReadFile(filepath.Join(path, "Contents", "Info.plist"))
	if err != nil {
		return model.ComputerApp{}, false
	}
	var info map[string]any
	if _, err := plist.Unmarshal(b, &info); err != nil {
		return model.ComputerApp{}, false
	}
	name := firstNonEmpty(str(info["CFBundleDisplayName"]), str(info["CFBundleName"]), strings.TrimSuffix(filepath.Base(path), ".app"))
	ver := firstNonEmpty(str(info["CFBundleShortVersionString"]), str(info["CFBundleVersion"]))
	return model.ComputerApp{Name: name, Version: ver, BundleID: str(info["CFBundleIdentifier"]), Path: path, Source: "agent"}, true
}

// CollectProfiles lists installed configuration profiles (computer level).
func CollectProfiles(ctx context.Context) []model.ComputerProfile {
	out := run(ctx, 30*time.Second, "profiles", "-C", "-o", "stdout-xml")
	if out == "" {
		return nil
	}
	var doc map[string][]map[string]any
	if _, err := plist.Unmarshal([]byte(out), &doc); err != nil {
		return nil
	}
	var profiles []model.ComputerProfile
	for _, items := range doc {
		for _, p := range items {
			profiles = append(profiles, model.ComputerProfile{
				Identifier: str(p["ProfileIdentifier"]), UUID: str(p["ProfileUUID"]), DisplayName: str(p["ProfileDisplayName"]),
				Organization: str(p["ProfileOrganization"]), IsManaged: strings.EqualFold(str(p["ProfileType"]), "Configuration") && str(p["ProfileInstallSource"]) == "mdm",
				Source: "agent",
			})
		}
	}
	return profiles
}

// CollectLocalUsers lists local accounts with uid >= 500 (attributes gathered concurrently).
func CollectLocalUsers(ctx context.Context) []model.LocalUser {
	out := run(ctx, 15*time.Second, "dscl", ".", "-list", "/Users", "UniqueID")
	type cand struct {
		name string
		uid  int
	}
	var cands []cand
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) != 2 {
			continue
		}
		uid, err := strconv.Atoi(f[1])
		if err != nil || uid < 500 || strings.HasPrefix(f[0], "_") {
			continue
		}
		cands = append(cands, cand{f[0], uid})
	}
	users := make([]model.LocalUser, len(cands))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for i, c := range cands {
		wg.Add(1)
		go func(i int, c cand) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			uid := c.uid
			u := model.LocalUser{Username: c.name, UID: &uid}
			u.RealName = dsclAttr(ctx, c.name, "RealName")
			u.Home = dsclAttr(ctx, c.name, "NFSHomeDirectory")
			u.IsAdmin = strings.Contains(run(ctx, 5*time.Second, "dsmemberutil", "checkmembership", "-U", c.name, "-G", "admin"), "is a member")
			st := runCombined(ctx, 5*time.Second, "sysadminctl", "-secureTokenStatus", c.name)
			if strings.Contains(st, "ENABLED") {
				u.SecureToken = boolp(true)
			} else if strings.Contains(st, "DISABLED") {
				u.SecureToken = boolp(false)
			}
			users[i] = u
		}(i, c)
	}
	wg.Wait()
	return users
}

// ParseEAResult extracts <result>…</result> if present.
func ParseEAResult(out string) string {
	out = strings.TrimSpace(out)
	if i := strings.Index(out, "<result>"); i >= 0 {
		if j := strings.Index(out[i:], "</result>"); j >= 0 {
			return strings.TrimSpace(out[i+len("<result>") : i+j])
		}
	}
	return out
}

var _ = bytes.NewReader
