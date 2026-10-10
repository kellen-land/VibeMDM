// Command vibemdm-sim simulates Macs enrolling in VibeMDM over the real MDM
// protocol (SCEP identity, Authenticate, TokenUpdate, command polling) and the
// real agent API. It is a development tool: no APNs, the fake devices poll.
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/smallstep/pkcs7"
	"github.com/smallstep/scep"
	"howett.net/plist"

	"github.com/vibemdm/vibemdm/internal/agent"
	"github.com/vibemdm/vibemdm/internal/model"
)

var version = "dev"

type device struct {
	idx      int
	udid     string
	serial   string
	name     string
	model    string
	osVer    string
	build    string
	key      *rsa.PrivateKey
	cert     *x509.Certificate
	profiles map[string]map[string]any // identifier -> payload dict
	violated map[string]bool           // restricted rule IDs already reported
	prkReady bool                      // a recovery key exists that macOS would escrow
	prkGen   int                       // regeneration counter (new key each time)
	scans    int                       // compliance scans run (drift appears on the 2nd)
	server   string
	topic    string
	client   *http.Client
	log      *slog.Logger
	agentTok string
}

func main() {
	server := flag.String("server", "http://localhost:8080", "VibeMDM server URL")
	n := flag.Int("n", 3, "number of simulated Macs")
	token := flag.String("token", "", "agent enrollment token (invitation or enrollment secret); empty = skip agent simulation")
	challenge := flag.String("scep-challenge", "", "SCEP challenge (from Settings → SCEP); required for MDM enrollment")
	topic := flag.String("topic", "com.apple.mgmt.External.simulator", "APNs topic to report in TokenUpdate")
	poll := flag.Duration("poll", 10*time.Second, "MDM command poll interval")
	checkin := flag.Duration("checkin", 30*time.Second, "agent check-in interval")
	prefix := flag.String("prefix", "SIM", "computer name prefix")
	noMDM := flag.Bool("no-mdm", false, "skip MDM enrollment (agent only)")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if !*noMDM && *challenge == "" {
		logger.Error("--scep-challenge is required for MDM enrollment (copy it from Settings → SCEP CA, or use --no-mdm)")
		os.Exit(2)
	}
	var wg sync.WaitGroup
	for i := 1; i <= *n; i++ {
		d := newDevice(i, *prefix, *server, *topic, logger)
		wg.Add(1)
		go func() {
			defer wg.Done()
			if !*noMDM {
				if err := d.enrollMDM(ctx, *challenge); err != nil {
					d.log.Error("mdm enrollment failed", "err", err)
					if *token == "" {
						return
					}
				} else {
					d.log.Info("mdm enrolled", "udid", d.udid, "serial", d.serial)
					go d.mdmLoop(ctx, *poll)
				}
			}
			if *token != "" {
				if err := d.enrollAgent(ctx, *token); err != nil {
					d.log.Error("agent enrollment failed", "err", err)
					return
				}
				d.agentLoop(ctx, *checkin)
			} else {
				<-ctx.Done()
			}
		}()
	}
	wg.Wait()
}

var models = []struct{ id, name string }{
	{"Mac14,2", "MacBook Air"}, {"Mac15,6", "MacBook Pro"}, {"Mac14,12", "Mac mini"}, {"Mac15,13", "MacBook Pro"}, {"Mac13,1", "Mac Studio"}, {"MacBookPro18,3", "MacBook Pro"},
}
var osVersions = []struct{ v, b string }{{"15.6.1", "24G90"}, {"15.5", "24F74"}, {"14.7.6", "23H626"}, {"26.0", "25A354"}}

func newDevice(i int, prefix, server, topic string, logger *slog.Logger) *device {
	m := models[i%len(models)]
	o := osVersions[i%len(osVersions)]
	serial := fmt.Sprintf("SIM%07d", 1000000+i*7919%9000000)
	udid := fmt.Sprintf("%08X-SIM0-4000-8000-%012d", 0x5A000000+i, i)
	d := &device{idx: i, prkReady: i%4 != 0, udid: udid, serial: serial, name: fmt.Sprintf("%s-Mac-%02d", prefix, i), model: m.id, osVer: o.v, build: o.b,
		profiles: map[string]map[string]any{}, violated: map[string]bool{}, server: strings.TrimRight(server, "/"), topic: topic,
		client: &http.Client{Timeout: 60 * time.Second}, log: logger.With("device", fmt.Sprintf("%s-Mac-%02d", prefix, i))}
	return d
}

// ---- SCEP ----

func (d *device) enrollMDM(ctx context.Context, challenge string) error {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}
	d.key = key
	// Fetch CA cert.
	resp, err := d.client.Get(d.server + "/scep?operation=GetCACert")
	if err != nil {
		return fmt.Errorf("GetCACert: %w", err)
	}
	caDER, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("GetCACert: HTTP %d", resp.StatusCode)
	}
	caCerts, err := scep.CACerts(caDER)
	if err != nil || len(caCerts) == 0 {
		ca, perr := x509.ParseCertificate(caDER)
		if perr != nil {
			return fmt.Errorf("parsing CA: %v / %v", err, perr)
		}
		caCerts = []*x509.Certificate{ca}
	}
	// CSR with challenge password.
	csrTmpl := &x509.CertificateRequest{
		Subject:            pkix.Name{CommonName: "VibeMDM Device Identity", Organization: []string{"VibeMDM"}},
		SignatureAlgorithm: x509.SHA256WithRSA,
	}
	csrDER, err := createCSRWithChallenge(csrTmpl, key, challenge)
	if err != nil {
		return err
	}
	csr, err := x509.ParseCertificateRequest(csrDER)
	if err != nil {
		return err
	}
	// Self-signed temporary cert to sign the SCEP request.
	selfTmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: csrTmpl.Subject, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment}
	selfDER, err := x509.CreateCertificate(rand.Reader, selfTmpl, selfTmpl, &key.PublicKey, key)
	if err != nil {
		return err
	}
	selfCert, _ := x509.ParseCertificate(selfDER)
	tmpl := &scep.PKIMessage{MessageType: scep.PKCSReq, Recipients: caCerts, SignerKey: key, SignerCert: selfCert}
	msg, err := scep.NewCSRRequest(csr, tmpl, scep.WithCACerts(caCerts))
	if err != nil {
		return fmt.Errorf("building PKCSReq: %w", err)
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, d.server+"/scep?operation=PKIOperation", bytes.NewReader(msg.Raw))
	req.Header.Set("Content-Type", "application/x-pki-message")
	resp, err = d.client.Do(req)
	if err != nil {
		return fmt.Errorf("PKIOperation: %w", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("PKIOperation: HTTP %d: %s", resp.StatusCode, body)
	}
	rep, err := scep.ParsePKIMessage(body, scep.WithCACerts(caCerts))
	if err != nil {
		return fmt.Errorf("parsing CertRep: %w", err)
	}
	if rep.PKIStatus != scep.SUCCESS {
		return fmt.Errorf("SCEP failed: status %s failinfo %s (wrong challenge?)", rep.PKIStatus, rep.FailInfo)
	}
	if err := rep.DecryptPKIEnvelope(selfCert, key); err != nil {
		return fmt.Errorf("decrypting CertRep: %w", err)
	}
	if rep.CertRepMessage == nil || rep.CertRepMessage.Certificate == nil {
		return fmt.Errorf("no certificate in CertRep")
	}
	d.cert = rep.CertRepMessage.Certificate
	d.log.Info("scep: identity issued", "serial", d.cert.SerialNumber.String())

	// Authenticate
	auth := map[string]any{
		"MessageType": "Authenticate", "Topic": d.topic, "UDID": d.udid, "SerialNumber": d.serial, "DeviceName": d.name,
		"Model": d.model, "ModelName": modelName(d.model), "OSVersion": d.osVer, "BuildVersion": d.build, "ProductName": d.model,
	}
	if err := d.checkin(ctx, auth); err != nil {
		return fmt.Errorf("Authenticate: %w", err)
	}
	// TokenUpdate
	tok := make([]byte, 32)
	rand.Read(tok)
	tu := map[string]any{
		"MessageType": "TokenUpdate", "Topic": d.topic, "UDID": d.udid, "Token": tok, "PushMagic": fmt.Sprintf("SIM-%d-%x", d.idx, tok[:4]),
		"AwaitingConfiguration": false,
	}
	if err := d.checkin(ctx, tu); err != nil {
		return fmt.Errorf("TokenUpdate: %w", err)
	}
	return nil
}

func modelName(id string) string {
	for _, m := range models {
		if m.id == id {
			return m.name
		}
	}
	return "Mac"
}

// createCSRWithChallenge builds a CSR carrying the SCEP challengePassword attribute.
func createCSRWithChallenge(tmpl *x509.CertificateRequest, key *rsa.PrivateKey, challenge string) ([]byte, error) {
	if challenge == "" {
		return x509.CreateCertificateRequest(rand.Reader, tmpl, key)
	}
	// challengePassword OID 1.2.840.113549.1.9.7 as a PKCS#9 attribute.
	tmpl.Attributes = nil
	tmpl.ExtraExtensions = nil
	der, err := x509.CreateCertificateRequest(rand.Reader, tmpl, key)
	if err != nil {
		return nil, err
	}
	return addChallengePassword(der, key, challenge)
}

// ---- MDM transport ----

// sign produces the Mdm-Signature header (detached CMS signature over body).
func (d *device) sign(body []byte) (string, error) {
	sd, err := pkcs7.NewSignedData(body)
	if err != nil {
		return "", err
	}
	sd.SetDigestAlgorithm(pkcs7.OIDDigestAlgorithmSHA256)
	if err := sd.AddSigner(d.cert, d.key, pkcs7.SignerInfoConfig{}); err != nil {
		return "", err
	}
	sd.Detach()
	sig, err := sd.Finish()
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(sig), nil
}

func (d *device) post(ctx context.Context, body []byte, checkin bool) ([]byte, int, error) {
	sig, err := d.sign(body)
	if err != nil {
		return nil, 0, err
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPut, d.server+"/mdm", bytes.NewReader(body))
	if checkin {
		req.Header.Set("Content-Type", "application/x-apple-aspen-mdm-checkin")
	} else {
		req.Header.Set("Content-Type", "application/x-apple-aspen-mdm")
	}
	req.Header.Set("Mdm-Signature", sig)
	req.Header.Set("User-Agent", "MDM/1.0 (simulator)")
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return out, resp.StatusCode, nil
}

func encodePlist(v any) []byte {
	var buf bytes.Buffer
	enc := plist.NewEncoder(&buf)
	enc.Indent("\t")
	enc.Encode(v)
	return buf.Bytes()
}

func (d *device) checkin(ctx context.Context, msg map[string]any) error {
	out, code, err := d.post(ctx, encodePlist(msg), true)
	if err != nil {
		return err
	}
	if code != 200 {
		return fmt.Errorf("HTTP %d: %s", code, strings.TrimSpace(string(out)))
	}
	return nil
}

// mdmLoop polls for commands and answers them.
func (d *device) mdmLoop(ctx context.Context, interval time.Duration) {
	report := map[string]any{"Status": "Idle", "UDID": d.udid}
	for ctx.Err() == nil {
		out, code, err := d.post(ctx, encodePlist(report), false)
		if err != nil || code != 200 {
			d.log.Warn("mdm poll failed", "err", err, "code", code)
			report = map[string]any{"Status": "Idle", "UDID": d.udid}
			sleep(ctx, interval)
			continue
		}
		if len(bytes.TrimSpace(out)) == 0 {
			report = map[string]any{"Status": "Idle", "UDID": d.udid}
			sleep(ctx, interval)
			continue
		}
		var cmd struct {
			CommandUUID string
			Command     map[string]any
		}
		if _, err := plist.Unmarshal(out, &cmd); err != nil {
			d.log.Warn("bad command", "err", err)
			sleep(ctx, interval)
			continue
		}
		rt, _ := cmd.Command["RequestType"].(string)
		d.log.Info("mdm command", "type", rt, "uuid", cmd.CommandUUID)
		report = d.respond(cmd.CommandUUID, rt, cmd.Command)
		// Immediately send the response (no sleep) — like a real device.
	}
}

func sleep(ctx context.Context, dur time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(dur):
	}
}

func (d *device) respond(uuid, rt string, cmd map[string]any) map[string]any {
	r := map[string]any{"Status": "Acknowledged", "UDID": d.udid, "CommandUUID": uuid}
	switch rt {
	case "DeviceInformation":
		r["QueryResponses"] = map[string]any{
			"UDID": d.udid, "DeviceName": d.name, "OSVersion": d.osVer, "BuildVersion": d.build, "ModelName": modelName(d.model), "Model": d.model,
			"ProductName": d.model, "SerialNumber": d.serial, "DeviceCapacity": 494.38, "AvailableDeviceCapacity": 210.5 - float64(d.idx*3),
			"BatteryLevel": 0.5 + float64(d.idx%5)/10, "IsSupervised": false, "IsActivationLockEnabled": false, "LocalHostName": strings.ReplaceAll(d.name, " ", "-"),
			"HostName": strings.ToLower(d.name) + ".local", "WiFiMAC": fmt.Sprintf("a4:83:e7:%02x:%02x:%02x", d.idx, d.idx*3, d.idx*7), "IsAppleSilicon": true,
			"SystemIntegrityProtectionEnabled": true, "TimeZone": "America/New_York", "ManagementStatus": map[string]any{"EnrolledViaDEP": false, "UserApprovedEnrollment": true},
		}
	case "SecurityInfo":
		sec := map[string]any{
			"FDE_Enabled": d.idx%2 == 0, "FDE_HasPersonalRecoveryKey": d.idx%2 == 0, "SystemIntegrityProtectionEnabled": true,
			"FirewallSettings":                       map[string]any{"FirewallEnabled": d.idx%3 != 0, "BlockAllIncoming": false, "StealthMode": false},
			"SecureBoot":                             map[string]any{"SecureBootLevel": "full", "ExternalBootLevel": "disallowed"},
			"BootstrapTokenAllowedForAuthentication": "allowed", "BootstrapTokenRequiredForSoftwareUpdate": true,
		}
		// Like macOS: with an FDERecoveryKeyEscrow profile installed and a
		// personal recovery key present, return it CMS-encrypted to the escrow cert.
		if d.idx%2 == 0 && d.prkReady {
			if cms := d.escrowedKeyCMS(); cms != nil {
				sec["FDE_PersonalRecoveryKeyCMS"] = cms
			}
		}
		r["SecurityInfo"] = sec
	case "ProfileList":
		list := []any{map[string]any{"PayloadIdentifier": "com.vibemdm.enrollment", "PayloadUUID": "1F2E3D4C-5B6A-4798-8A9B-0C1D2E3F4A5B", "PayloadDisplayName": "VibeMDM Enrollment", "PayloadOrganization": "VibeMDM", "IsManaged": true}}
		for id, p := range d.profiles {
			list = append(list, map[string]any{"PayloadIdentifier": id, "PayloadUUID": str(p["PayloadUUID"]), "PayloadDisplayName": str(p["PayloadDisplayName"]), "PayloadOrganization": str(p["PayloadOrganization"]), "IsManaged": true})
		}
		r["ProfileList"] = list
	case "InstalledApplicationList":
		r["InstalledApplicationList"] = []any{
			map[string]any{"Name": "Safari", "ShortVersion": "18.6", "Identifier": "com.apple.Safari", "BundleSize": 0},
			map[string]any{"Name": "Google Chrome", "ShortVersion": "140.0.7339.80", "Identifier": "com.google.Chrome"},
			map[string]any{"Name": "Slack", "ShortVersion": fmt.Sprintf("4.4%d.0", d.idx%5), "Identifier": "com.tinyspeck.slackmacgap"},
			map[string]any{"Name": "Visual Studio Code", "ShortVersion": "1.104.0", "Identifier": "com.microsoft.VSCode"},
		}
	case "InstallProfile":
		if payload, ok := cmd["Payload"].([]byte); ok {
			var p map[string]any
			if _, err := plist.Unmarshal(payload, &p); err == nil {
				id := str(p["PayloadIdentifier"])
				d.profiles[id] = p
				d.log.Info("installed profile", "identifier", id)
			} else {
				r["Status"] = "Error"
				r["ErrorChain"] = []any{map[string]any{"ErrorCode": 4000, "ErrorDomain": "MCProfileErrorDomain", "LocalizedDescription": "The profile is not valid."}}
			}
		}
	case "RemoveProfile":
		id := str(cmd["Identifier"])
		if _, ok := d.profiles[id]; ok {
			delete(d.profiles, id)
		} else if id != "com.vibemdm.enrollment" {
			r["Status"] = "Error"
			r["ErrorChain"] = []any{map[string]any{"ErrorCode": 4001, "ErrorDomain": "MCProfileErrorDomain", "LocalizedDescription": "The profile “" + id + "” is not installed."}}
		}
	case "CertificateList":
		r["CertificateList"] = []any{map[string]any{"CommonName": "VibeMDM Device Identity", "IsIdentity": true, "Data": d.cert.Raw}}
	case "DeviceLock", "EraseDevice", "RestartDevice", "ShutDownDevice", "EnableRemoteDesktop", "DisableRemoteDesktop", "DeviceConfigured":
		// acknowledged
	case "DeclarativeManagement":
		go d.ddmSync()
	case "AvailableOSUpdates":
		r["AvailableOSUpdates"] = []any{map[string]any{"ProductKey": "MSU_UPDATE_26.0.1", "HumanReadableName": "macOS 26.0.1", "Version": "26.0.1", "IsCritical": false}}
	default:
		// Unknown commands: acknowledge anyway.
	}
	return r
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

// ---- agent simulation ----

func (d *device) enrollAgent(ctx context.Context, token string) error {
	c := agent.NewClient(d.server, "", version)
	resp, err := c.Enroll(ctx, model.AgentEnrollRequest{Token: token, SerialNumber: d.serial, UDID: d.udid, Hostname: strings.ToLower(d.name) + ".local",
		ComputerName: d.name, OSVersion: d.osVer, Architecture: "arm64", AgentVersion: version})
	if err != nil {
		return err
	}
	d.agentTok = resp.DeviceToken
	d.log.Info("agent enrolled", "computer_id", resp.ComputerID)
	return nil
}

func (d *device) agentLoop(ctx context.Context, interval time.Duration) {
	c := agent.NewClient(d.server, d.agentTok, version)
	user := []string{"alice", "bob", "carol", "dave"}[d.idx%4]
	runCheckin := func(trigger string) {
		defer c.ShipLogs(ctx, []model.AgentLogEntry{{At: time.Now().UTC(), Level: "INFO", Component: "daemon", Msg: "check-in complete",
			Attrs: map[string]any{"trigger": trigger, "simulated": true, "user": user}}})
		resp, err := c.Checkin(ctx, model.AgentCheckinRequest{Trigger: trigger, ConsoleUser: user, Hostname: strings.ToLower(d.name) + ".local", ComputerName: d.name,
			OSVersion: d.osVer, OSBuild: d.build, IPAddress: fmt.Sprintf("10.0.%d.%d", d.idx/250, d.idx%250+10), AgentVersion: version, UptimeSecs: int64(3600 * d.idx)})
		if err != nil {
			d.log.Warn("agent checkin failed", "err", err)
			return
		}
		for _, p := range resp.Policies {
			d.runPolicy(ctx, c, p)
		}
		for _, t := range resp.Tasks {
			d.runTask(ctx, c, t)
		}
		for _, app := range resp.AppInstalls {
			d.log.Info("simulating app install", "app", app.Name, "label", app.Label)
			c.SubmitAppInstallResult(ctx, app.LogID, "completed", "[simulator] Installomator "+strings.Join(app.Args, " ")+": installed")
		}
		if n := len(resp.RestrictedSoftware); n > 0 && d.idx == 1 {
			// First simulated Mac "violates" each rule once so the console has data.
			for _, ru := range resp.RestrictedSoftware {
				if _, done := d.violated[ru.ID]; done {
					continue
				}
				d.violated[ru.ID] = true
				c.ReportRestricted(ctx, model.AgentRestrictedReport{RuleID: ru.ID, RuleName: ru.Name, Process: ru.ProcessName, Username: user, Killed: ru.KillProcess})
			}
		}
		if resp.InventoryRequested {
			d.inventory(ctx, c, user)
		}
	}
	runCheckin("startup")
	runCheckin("checkin")
	go func() {
		for ctx.Err() == nil {
			resp, err := c.PollTasks(ctx, 50)
			if err != nil {
				sleep(ctx, 5*time.Second)
				continue
			}
			for _, t := range resp.Tasks {
				d.runTask(ctx, c, t)
			}
			if resp.Checkin {
				d.log.Info("server requested immediate check-in")
				runCheckin("checkin")
			}
			for _, inv := range resp.Shells {
				go d.handleShell(ctx, inv)
			}
		}
	}()
	for ctx.Err() == nil {
		sleep(ctx, interval)
		if ctx.Err() == nil {
			runCheckin("checkin")
		}
	}
}

func (d *device) runPolicy(ctx context.Context, c *agent.Client, p model.AgentPolicy) model.AgentPolicyLogSubmit {
	d.log.Info("simulating policy", "policy", p.Name)
	sub := model.AgentPolicyLogSubmit{Trigger: p.Trigger, StartedAt: time.Now().UTC(), Status: "completed"}
	var logb strings.Builder
	fmt.Fprintf(&logb, "Simulated execution of policy %q\n", p.Name)
	for _, s := range p.Scripts {
		exit := 0
		out := fmt.Sprintf("[simulator] ran script %q with params %v\n", s.Name, s.Parameters)
		if strings.Contains(strings.ToLower(s.Name), "fail") {
			exit, out = 1, out+"[simulator] script name contains 'fail' — failing on purpose\n"
			sub.Status = "failed"
		}
		sub.Steps = append(sub.Steps, model.PolicyStep{Type: "script", Name: s.Name, Status: statusFor(exit), ExitCode: exit, Output: out, DurationMS: 120 + int64(d.idx)})
		fmt.Fprintf(&logb, "script %q exit %d\n", s.Name, exit)
	}
	for _, pk := range p.Packages {
		sub.Steps = append(sub.Steps, model.PolicyStep{Type: "package", Name: pk.Name, Status: "completed", Output: "[simulator] installer: The install was successful.", DurationMS: 2500})
		fmt.Fprintf(&logb, "package %q installed\n", pk.Name)
	}
	for _, a := range p.Apps {
		sub.Steps = append(sub.Steps, model.PolicyStep{Type: "app", Name: a.Name, Status: "completed", Output: "[simulator] Installomator " + strings.Join(a.Args, " "), DurationMS: 4100})
		fmt.Fprintf(&logb, "Installomator %q installed\n", a.Name)
	}
	if p.RunCommand != "" {
		sub.Steps = append(sub.Steps, model.PolicyStep{Type: "command", Name: "Execute Command", Status: "completed", Output: "[simulator] " + p.RunCommand, DurationMS: 40})
	}
	if p.UpdateInventory {
		d.inventory(ctx, c, "")
		sub.Steps = append(sub.Steps, model.PolicyStep{Type: "inventory", Name: "Update Inventory", Status: "completed", Output: "inventory submitted", DurationMS: 900})
	}
	sub.CompletedAt = time.Now().UTC()
	sub.Log = logb.String()
	if p.LogID != "" {
		if err := c.SubmitPolicyLog(ctx, p.LogID, sub); err != nil {
			d.log.Warn("submit policy log", "err", err)
		}
	}
	return sub
}

func fnv32(s string) uint32 {
	h := uint32(2166136261)
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return h
}

func statusFor(exit int) string {
	if exit == 0 {
		return "completed"
	}
	return "failed"
}

func (d *device) runTask(ctx context.Context, c *agent.Client, t model.AgentTask) {
	d.log.Info("simulating task", "type", t.Type)
	res := model.AgentTaskResult{StartedAt: time.Now().UTC(), Status: "completed"}
	switch t.Type {
	case "run_script":
		name := str(t.Payload["name"])
		res.Stdout = fmt.Sprintf("[simulator] ran script %q\n", name)
		if strings.Contains(strings.ToLower(name), "fail") {
			res.Status, res.ExitCode, res.Stderr = "failed", 1, "simulated failure"
		}
	case "run_command":
		res.Stdout = "[simulator] $ " + str(t.Payload["command"]) + "\n(simulated output)\n"
	case "run_policy":
		var ap model.AgentPolicy
		b, _ := jsonRoundTrip(t.Payload["policy"])
		_ = b
		if ap2, ok := decodePolicy(t.Payload["policy"]); ok {
			ap = ap2
		}
		sub := d.runPolicy(ctx, c, ap)
		res.Stdout = sub.Log
		if sub.Status != "completed" {
			res.Status = "failed"
		}
	case "inventory":
		d.inventory(ctx, c, "")
		res.Stdout = "inventory submitted"
	case "custom_trigger":
		res.Stdout = "[simulator] trigger " + str(t.Payload["trigger"])
	case "collect_diagnostics":
		id := str(t.Payload["bundle_id"])
		path := fmt.Sprintf("/tmp/vibemdm-sim-diag-%s.tar.gz", id)
		if err := writeFakeBundle(path, d.name); err != nil {
			res.Status, res.Error = "failed", err.Error()
			c.DiagnosticsFailed(ctx, id, err.Error())
			break
		}
		if err := c.UploadDiagnostics(ctx, id, path); err != nil {
			res.Status, res.Error = "failed", err.Error()
		} else {
			res.Stdout = "[simulator] diagnostics uploaded"
		}
		os.Remove(path)
	case "compliance_scan":
		var scan model.AgentComplianceScan
		if b, err := json.Marshal(t.Payload["scan"]); err == nil {
			json.Unmarshal(b, &scan)
		}
		d.scans++
		sub := model.AgentComplianceSubmit{OSVersion: d.osVer, DurationMS: int64(40 * len(scan.Rules))}
		counts := map[string]int{}
		for i, r := range scan.Rules {
			// Stable per-Mac failures (~12%), plus one rule that drifts on the second scan.
			h := fnv32(d.name + r.ID)
			fail := h%100 < 12 || (d.scans >= 2 && i == d.idx)
			rr := model.AgentComplianceRuleResult{ID: r.ID, Status: "pass", Actual: r.Expected}
			if fail {
				rr.Status, rr.Actual = "fail", "simulated-noncompliant"
				if scan.Remediate && r.Fix != "" && h%2 == 0 {
					rr.Status, rr.Actual, rr.Detail = "remediated", r.Expected, "fixed by the mSCP fix script"
				}
			}
			counts[rr.Status]++
			sub.Results = append(sub.Results, rr)
		}
		if err := c.SubmitComplianceScan(ctx, scan.ScanID, sub); err != nil {
			res.Status, res.Error = "failed", err.Error()
			break
		}
		res.Stdout = fmt.Sprintf("[simulator] %s: %d passed, %d failed, %d remediated", scan.Baseline, counts["pass"], counts["fail"], counts["remediated"])
	case "filevault_regenerate":
		if d.idx%2 != 0 {
			res.Status, res.ExitCode, res.Stdout = "failed", 1, "FileVault is not enabled on this Mac; nothing to escrow."
			break
		}
		d.prkReady, d.prkGen = true, d.prkGen+1 // simulate Escrow Buddy + next login
		res.Stdout = "[simulator] Escrow Buddy armed; simulated login generated a new recovery key."
	default:
		res.Stdout = "[simulator] " + t.Type
	}
	res.CompletedAt = time.Now().UTC()
	if err := c.SubmitTaskResult(ctx, t.ID, res); err != nil {
		d.log.Warn("submit task result", "err", err)
	}
}

func (d *device) inventory(ctx context.Context, c *agent.Client, user string) {
	if user == "" {
		user = []string{"alice", "bob", "carol", "dave"}[d.idx%4]
	}
	rc, _ := c.ReconConfig(ctx)
	eas := map[string]string{}
	if rc != nil {
		for _, ea := range rc.ExtensionAttributes {
			eas[ea.ID] = fmt.Sprintf("sim-%s-%d", strings.ToLower(strings.ReplaceAll(ea.Name, " ", "-")), d.idx)
		}
	}
	fv := d.idx%2 == 0
	tr := true
	batt := 40 + d.idx*7%60
	inv := model.AgentInventory{
		AgentVersion: version, ComputerName: d.name, Hostname: strings.ToLower(d.name) + ".local", LocalHostname: d.name, SerialNumber: d.serial, UDID: d.udid,
		ModelIdentifier: d.model, ModelName: modelName(d.model), OSVersion: d.osVer, OSBuild: d.build, Architecture: "arm64", CPU: "Apple M3 Pro", CPUCores: 12,
		MemoryMB: 36 * 1024, DiskTotalMB: 494 * 1024, DiskFreeMB: int64(200-d.idx*5) * 1024, IPAddress: fmt.Sprintf("10.0.%d.%d", d.idx/250, d.idx%250+10),
		MACAddress: fmt.Sprintf("a4:83:e7:%02x:%02x:%02x", d.idx, d.idx*3, d.idx*7), ConsoleUser: user, ConsoleUserFullName: strings.Title(user) + " Example", LastUser: user,
		FileVaultEnabled: &fv, SIPEnabled: &tr, FirewallEnabled: &tr, GatekeeperEnabled: &tr, SecureBootLevel: "Full Security", BatteryPercent: &batt, UptimeSeconds: int64(3600 * d.idx),
		Apps: []model.ComputerApp{
			{Name: "Safari", Version: "18.6", BundleID: "com.apple.Safari", Path: "/Applications/Safari.app"},
			{Name: "Google Chrome", Version: "140.0.7339.80", BundleID: "com.google.Chrome", Path: "/Applications/Google Chrome.app"},
			{Name: "Slack", Version: fmt.Sprintf("4.4%d.0", d.idx%5), BundleID: "com.tinyspeck.slackmacgap", Path: "/Applications/Slack.app"},
			{Name: "Visual Studio Code", Version: "1.104.0", BundleID: "com.microsoft.VSCode", Path: "/Applications/Visual Studio Code.app"},
			{Name: "zoom.us", Version: "6.5.3", BundleID: "us.zoom.xos", Path: "/Applications/zoom.us.app"},
		},
		LocalUsers: []model.LocalUser{{Username: user, UID: intp(501), RealName: strings.Title(user) + " Example", Home: "/Users/" + user, IsAdmin: true, SecureToken: &tr},
			{Username: "itadmin", UID: intp(502), RealName: "IT Admin", Home: "/Users/itadmin", IsAdmin: true, SecureToken: &tr}},
		ExtensionAttributes: eas, Hardware: map[string]any{"chip_type": "Apple M3 Pro", "machine_name": modelName(d.model), "physical_memory": "36 GB", "serial_number": d.serial},
		Extra: map[string]any{"simulator": true}, CollectedAt: time.Now().UTC(),
	}
	if len(d.profiles) > 0 {
		for id, p := range d.profiles {
			inv.Profiles = append(inv.Profiles, model.ComputerProfile{Identifier: id, UUID: str(p["PayloadUUID"]), DisplayName: str(p["PayloadDisplayName"]), IsManaged: true})
		}
	}
	if err := c.SubmitInventory(ctx, inv); err != nil {
		d.log.Warn("inventory failed", "err", err)
	} else {
		d.log.Info("inventory submitted")
	}
}

func intp(n int) *int { return &n }
