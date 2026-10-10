package health

import (
	"context"
	"crypto/x509"
	"database/sql"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/vibemdm/vibemdm/internal/escrow"
	"github.com/vibemdm/vibemdm/internal/model"
	"github.com/vibemdm/vibemdm/internal/store"
)

// Commander is the subset of the MDM commander remediations use.
type Commander interface {
	PushComputer(ctx context.Context, comp *model.Computer) error
	InstallProfile(ctx context.Context, comp *model.Computer, prof *model.ConfigProfile, createdBy string) error
	SyncDDM(ctx context.Context, comp *model.Computer, createdBy string) error
}

// Deps are what the built-in checks need.
type Deps struct {
	St                *store.Store
	Cmdr              Commander
	EncryptionEnabled bool
	// Wake nudges a long-polling agent to fetch new tasks now (optional).
	Wake func(computerID string)
	// DroppedLogs reports log records the DB sink had to drop (optional).
	DroppedLogs func() int64
}

const actor = "health"

// Built-in check IDs.
const (
	CheckAPNsCert          = "platform.apns_certificate"
	CheckServerURL         = "platform.server_url"
	CheckEncryption        = "security.encryption_at_rest"
	CheckSCEPCA            = "platform.scep_ca"
	CheckSigningCert       = "platform.signing_certificate"
	CheckPushFailures      = "platform.push_failures"
	CheckWebhooks          = "platform.webhook_failures"
	CheckRemoteShellHTTP   = "security.remote_shell_transport"
	CheckMDMUnresponsive   = "fleet.mdm_unresponsive"
	CheckAgentSilent       = "fleet.agent_silent"
	CheckAgentMissing      = "fleet.agent_missing"
	CheckDiskSpace         = "fleet.disk_space"
	CheckSecurityPosture   = "security.device_posture"
	CheckFileVaultNoKey    = "security.filevault_key_missing"
	CheckFileVaultNoEscrow = "security.filevault_escrow_profile"
	CheckProfileFailures   = "configuration.profile_failures"
	CheckDDMInvalid        = "configuration.ddm_invalid"
	CheckDDMStuck          = "configuration.ddm_not_syncing"
	CheckPolicyFailures    = "configuration.policy_failures"
	CheckAppFailures       = "configuration.app_install_failures"
	CheckPolicyNeverRuns   = "configuration.policy_never_runs"
	CheckStaleScope        = "configuration.stale_scope_references"
)

// Builtin returns the registry of built-in checks.
//
// Thresholds are deliberately conservative: each check describes a condition
// an administrator would want to act on, not every anomaly.
func Builtin(d Deps) *Registry {
	r := NewRegistry()
	c := &checks{d: d}
	r.Add(Check{ID: CheckAPNsCert, Category: "platform", Title: "APNs push certificate",
		Description: "Missing, expiring (30 days) or expired APNs certificate. When it expires no Mac can be reached until it is renewed with the same Apple ID.",
		Run:         c.apnsCert})
	r.Add(Check{ID: CheckServerURL, Category: "platform", Title: "Server URL",
		Description: "The public server URL must be set and use HTTPS; Macs refuse MDM servers over plain HTTP.",
		Run:         c.serverURL})
	r.Add(Check{ID: CheckEncryption, Category: "security", Title: "Encryption at rest",
		Description: "VIBEMDM_ENCRYPTION_KEY is not set, so private keys and FileVault recovery keys are stored unencrypted.",
		Run:         c.encryption})
	r.Add(Check{ID: CheckSCEPCA, Category: "platform", Title: "SCEP CA certificate",
		Description: "The built-in CA that issues device identities expires within 90 days.",
		Run:         c.scepCA})
	r.Add(Check{ID: CheckSigningCert, Category: "platform", Title: "Profile signing certificate",
		Description: "The enrollment profile signing certificate is invalid or expires within 30 days.",
		Run:         c.signingCert})
	r.Add(Check{ID: CheckPushFailures, Category: "platform", Title: "APNs push failures",
		Description: "Most pushes in the last hour failed. Commands will not be delivered until APNs is reachable.",
		Grace:       10 * time.Minute, Run: c.pushFailures})
	r.Add(Check{ID: CheckWebhooks, Category: "platform", Title: "Failing webhooks",
		Description: "An enabled webhook's last five deliveries all failed.",
		Run:         c.webhooks})
	r.Add(Check{ID: CheckRemoteShellHTTP, Category: "security", Title: "Remote shell over plain HTTP",
		Description: "Remote shell is enabled while the server URL is not HTTPS, so sessions could be intercepted.",
		Run:         c.remoteShell})
	r.Add(Check{ID: CheckMDMUnresponsive, Category: "fleet", Title: "MDM channel not responding",
		Description:  "The agent is checking in but MDM commands have waited over 2 hours. Usually APNs is blocked on the Mac's network, or the push token is stale.",
		SupportsHeal: true, AutoHealDefault: true, Grace: 30 * time.Minute, Run: c.mdmUnresponsive})
	r.Add(Check{ID: CheckAgentSilent, Category: "fleet", Title: "Agent not checking in",
		Description: "MDM is checking in but the VibeMDM agent has been silent for over 3 check-in intervals (min. 2 hours). The launch daemon may be unloaded or the agent removed.",
		Grace:       30 * time.Minute, Run: c.agentSilent})
	r.Add(Check{ID: CheckAgentMissing, Category: "fleet", Title: "Agent not installed",
		Description: "The Mac enrolled in MDM over 24 hours ago but the agent never enrolled, so scripts, policies and inventory won't run.",
		Run:         c.agentMissing})
	r.Add(Check{ID: CheckDiskSpace, Category: "fleet", Title: "Low disk space",
		Description: "Less than 10 GB (warning) or 5 GB (critical) free. macOS updates need roughly 25 GB.",
		Run:         c.diskSpace})
	r.Add(Check{ID: CheckSecurityPosture, Category: "security", Title: "Security features disabled",
		Description: "System Integrity Protection or Gatekeeper is turned off.",
		Run:         c.securityPosture})
	r.Add(Check{ID: CheckFileVaultNoKey, Category: "security", Title: "FileVault key not escrowed",
		Description:  "FileVault is on, the escrow profile is installed, but no recovery key has been escrowed for 24 hours. Escrow Buddy can generate a new key at next login.",
		SupportsHeal: true, Run: c.fileVaultNoKey})
	r.Add(Check{ID: CheckFileVaultNoEscrow, Category: "security", Title: "FileVault escrow not configured",
		Description: "Macs have FileVault on but the recovery key escrow profile is not deployed to them, so keys cannot be recovered.",
		Run:         c.fileVaultNoEscrow})
	r.Add(Check{ID: CheckProfileFailures, Category: "configuration", Title: "Configuration profile failures",
		Description:  "A profile failed to install on one or more Macs.",
		SupportsHeal: true, Run: c.profileFailures})
	r.Add(Check{ID: CheckDDMInvalid, Category: "configuration", Title: "Invalid declarations",
		Description: "A Mac reports a declaration as invalid (for example unsupported on its macOS version or enrollment type).",
		Grace:       15 * time.Minute, Run: c.ddmInvalid})
	r.Add(Check{ID: CheckDDMStuck, Category: "configuration", Title: "Declarations not syncing",
		Description:  "A Mac was asked to sync declarations over 6 hours ago and has not fetched them.",
		SupportsHeal: true, AutoHealDefault: true, Run: c.ddmStuck})
	r.Add(Check{ID: CheckPolicyFailures, Category: "configuration", Title: "Failing policies",
		Description: "A policy's latest run failed on at least 3 Macs (and half or more of those it ran on) in the last 7 days.",
		Run:         c.policyFailures})
	r.Add(Check{ID: CheckAppFailures, Category: "configuration", Title: "Failing app installs",
		Description: "An app's latest Installomator run failed on at least 3 Macs (and half or more) in the last 7 days. The label may be broken or the vendor download changed.",
		Run:         c.appFailures})
	r.Add(Check{ID: CheckPolicyNeverRuns, Category: "configuration", Title: "Policy can never run",
		Description: "An enabled policy has no trigger and is not in Self Service, or has nothing to do.",
		Run:         c.policyNeverRuns})
	r.Add(Check{ID: CheckStaleScope, Category: "configuration", Title: "Scope references deleted objects",
		Description: "A policy, profile, app or update plan scope names a group, computer or network segment that no longer exists.",
		Run:         c.staleScope})
	return r
}

type checks struct{ d Deps }

func (c *checks) db() *sql.DB { return c.d.St.DB }

func parseCert(pemText string) (*x509.Certificate, error) {
	b, _ := pem.Decode([]byte(pemText))
	if b == nil {
		return nil, errors.New("not PEM")
	}
	return x509.ParseCertificate(b.Bytes)
}

func days(d time.Duration) int { return int(d.Hours() / 24) }

// expiryFinding grades a certificate's remaining lifetime.
func expiryFinding(cert *x509.Certificate, warnDays, critDays int) (severity, summary string, ok bool) {
	left := time.Until(cert.NotAfter)
	switch {
	case left <= 0:
		return Critical, fmt.Sprintf("expired %d days ago", -days(left)), true
	case left < time.Duration(critDays)*24*time.Hour:
		return Critical, fmt.Sprintf("expires in %d days", days(left)), true
	case left < time.Duration(warnDays)*24*time.Hour:
		return Warning, fmt.Sprintf("expires in %d days", days(left)), true
	}
	return "", "", false
}

func link(id, label, desc, url string) Remediation {
	return Remediation{ID: id, Label: label, Description: desc, URL: url}
}

// ---- platform ----

func (c *checks) apnsCert(ctx context.Context) ([]Finding, error) {
	topics, err := c.d.St.GetPushCertTopics(ctx)
	if err != nil {
		return nil, err
	}
	fix := link("renew", "Renew the APNs certificate", "Create a CSR in Settings, renew it at identity.apple.com with the same Apple ID, then upload it.", "/settings/apns")
	if len(topics) == 0 {
		return []Finding{{ObjectType: "server", ObjectID: "apns", ObjectName: "APNs certificate", Severity: Warning,
			Summary: "No APNs push certificate uploaded", Detail: "Macs cannot enroll in MDM until a push certificate is uploaded.",
			Remediations: []Remediation{fix}}}, nil
	}
	var out []Finding
	for _, t := range topics {
		p, err := c.d.St.GetPushCertPEM(ctx, t)
		if err != nil {
			return nil, err
		}
		cert, err := parseCert(p)
		if err != nil {
			continue
		}
		if sev, sum, bad := expiryFinding(cert, 30, 7); bad {
			out = append(out, Finding{ObjectType: "server", ObjectID: "apns:" + t, ObjectName: "APNs certificate", Severity: sev,
				Summary: "APNs certificate " + sum,
				Detail:  "Renew (do not replace) the certificate with the same Apple ID so the topic stays the same; a new topic requires re-enrolling every Mac.",
				Facts:   map[string]any{"topic": t, "not_after": cert.NotAfter}, Remediations: []Remediation{fix}})
		}
	}
	return out, nil
}

func (c *checks) serverURL(ctx context.Context) ([]Finding, error) {
	s, err := c.d.St.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	fix := link("settings", "Set the server URL", "Use the public HTTPS address Macs reach this server at (behind a TLS reverse proxy).", "/settings/general")
	if s.ServerURL == "" {
		return []Finding{{ObjectType: "server", ObjectID: "server_url", ObjectName: "Server URL", Severity: Critical,
			Summary: "Server URL is not set", Detail: "Enrollment profiles and the agent installer embed this URL.", Remediations: []Remediation{fix}}}, nil
	}
	u, err := url.Parse(s.ServerURL)
	if err != nil || u.Host == "" {
		return []Finding{{ObjectType: "server", ObjectID: "server_url", ObjectName: "Server URL", Severity: Critical,
			Summary: "Server URL is not a valid URL", Facts: map[string]any{"server_url": s.ServerURL}, Remediations: []Remediation{fix}}}, nil
	}
	if u.Scheme != "https" {
		return []Finding{{ObjectType: "server", ObjectID: "server_url", ObjectName: "Server URL", Severity: Warning,
			Summary: "Server URL does not use HTTPS", Detail: "macOS requires HTTPS for the MDM check-in and server URLs, so real Macs cannot enroll.",
			Facts: map[string]any{"server_url": s.ServerURL}, Remediations: []Remediation{fix}}}, nil
	}
	return nil, nil
}

func (c *checks) encryption(ctx context.Context) ([]Finding, error) {
	if c.d.EncryptionEnabled {
		return nil, nil
	}
	return []Finding{{ObjectType: "server", ObjectID: "encryption", ObjectName: "Secrets storage", Severity: Warning,
		Summary:      "Secrets are stored unencrypted",
		Detail:       "Set VIBEMDM_ENCRYPTION_KEY to a long random value and restart; existing secrets are encrypted automatically on start-up.",
		Remediations: []Remediation{link("docs", "How to enable encryption", "Add VIBEMDM_ENCRYPTION_KEY (e.g. openssl rand -base64 32) to the server environment and restart. See docs/architecture.md → Security.", "")}}}, nil
}

func (c *checks) scepCA(ctx context.Context) ([]Finding, error) {
	certPEM, _, err := c.d.St.GetSCEPCA(ctx)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	cert, err := parseCert(certPEM)
	if err != nil {
		return nil, err
	}
	if sev, sum, bad := expiryFinding(cert, 90, 14); bad {
		return []Finding{{ObjectType: "server", ObjectID: "scep_ca", ObjectName: "SCEP CA", Severity: sev,
			Summary: "SCEP CA certificate " + sum,
			Detail:  "Device identity certificates chain to this CA; when it expires Macs can no longer authenticate to MDM.",
			Facts:   map[string]any{"not_after": cert.NotAfter}}}, nil
	}
	return nil, nil
}

func (c *checks) signingCert(ctx context.Context) ([]Finding, error) {
	s, err := c.d.St.GetSettings(ctx)
	if err != nil || s.SigningCertPEM == "" {
		return nil, err
	}
	fix := link("settings", "Replace the signing certificate", "", "/settings/general")
	cert, err := parseCert(s.SigningCertPEM)
	if err != nil {
		return []Finding{{ObjectType: "server", ObjectID: "signing_cert", ObjectName: "Profile signing certificate", Severity: Warning,
			Summary: "Profile signing certificate cannot be parsed", Remediations: []Remediation{fix}}}, nil
	}
	if sev, sum, bad := expiryFinding(cert, 30, 7); bad {
		return []Finding{{ObjectType: "server", ObjectID: "signing_cert", ObjectName: "Profile signing certificate", Severity: sev,
			Summary: "Profile signing certificate " + sum, Facts: map[string]any{"subject": cert.Subject.CommonName, "not_after": cert.NotAfter},
			Remediations: []Remediation{fix}}}, nil
	}
	return nil, nil
}

func (c *checks) pushFailures(ctx context.Context) ([]Finding, error) {
	var sent, failed int
	var lastErr sql.NullString
	err := c.db().QueryRowContext(ctx, `SELECT count(*) FILTER (WHERE event = 'push_sent'), count(*) FILTER (WHERE event = 'push_failed'),
		(SELECT details->>'error' FROM mdm_command_events WHERE event = 'push_failed' ORDER BY at DESC LIMIT 1)
		FROM mdm_command_events WHERE at > now() - interval '1 hour' AND event IN ('push_sent','push_failed')`).Scan(&sent, &failed, &lastErr)
	if err != nil {
		return nil, err
	}
	if failed < 3 || failed*2 < sent+failed {
		return nil, nil
	}
	sev := Warning
	if sent == 0 {
		sev = Critical
	}
	return []Finding{{ObjectType: "server", ObjectID: "apns_push", ObjectName: "APNs", Severity: sev,
		Summary:      fmt.Sprintf("%d of %d pushes failed in the last hour", failed, sent+failed),
		Detail:       "Check outbound access to api.push.apple.com:443 and that the APNs certificate is valid. Last error: " + lastErr.String,
		Facts:        map[string]any{"sent": sent, "failed": failed, "last_error": lastErr.String},
		Remediations: []Remediation{link("logs", "Open server logs", "Filter to the push component.", "/logs")}}}, nil
}

func (c *checks) webhooks(ctx context.Context) ([]Finding, error) {
	rows, err := c.db().QueryContext(ctx, `SELECT w.id, w.name, d.failed, d.last_error, d.last_at FROM webhooks w
		JOIN LATERAL (SELECT bool_and(status < 200 OR status >= 300) AS failed, count(*) AS n,
			(array_agg(error ORDER BY at DESC))[1] AS last_error, max(at) AS last_at
			FROM (SELECT status, error, at FROM webhook_deliveries WHERE webhook_id = w.id ORDER BY at DESC LIMIT 5) x) d ON TRUE
		WHERE w.enabled AND d.n = 5 AND d.failed AND d.last_at > now() - interval '24 hours'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Finding
	for rows.Next() {
		var id, name, lastErr string
		var failed bool
		var last time.Time
		if err := rows.Scan(&id, &name, &failed, &lastErr, &last); err != nil {
			return nil, err
		}
		hookID := id
		out = append(out, Finding{ObjectType: "webhook", ObjectID: id, ObjectName: name, Severity: Warning,
			Summary: "Webhook " + name + " is failing", Detail: "The last 5 deliveries failed. Last error: " + lastErr,
			Facts: map[string]any{"last_error": lastErr, "last_attempt": last},
			Remediations: []Remediation{
				link("edit", "Review the webhook", "", "/settings/webhooks"),
				{ID: "disable", Label: "Disable the webhook", Description: "Stops further delivery attempts until it is fixed.",
					Run: func(ctx context.Context) error {
						_, err := c.db().ExecContext(ctx, `UPDATE webhooks SET enabled = FALSE, updated_at = now() WHERE id = $1`, hookID)
						return err
					}},
			}})
	}
	return out, rows.Err()
}

func (c *checks) remoteShell(ctx context.Context) ([]Finding, error) {
	s, err := c.d.St.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	if !s.RemoteShellEnabled || strings.HasPrefix(s.ServerURL, "https://") {
		return nil, nil
	}
	return []Finding{{ObjectType: "server", ObjectID: "remote_shell", ObjectName: "Remote shell", Severity: Warning,
		Summary: "Remote shell is enabled without HTTPS",
		Detail:  "Root shell sessions travel over an unencrypted WebSocket. Put the server behind TLS or turn remote shell off.",
		Remediations: []Remediation{
			link("settings", "Open settings", "", "/settings/general"),
			{ID: "disable", Label: "Turn remote shell off", Description: "Disables new remote shell sessions server-wide.",
				Run: func(ctx context.Context) error {
					s, err := c.d.St.GetSettings(ctx)
					if err != nil {
						return err
					}
					s.RemoteShellEnabled = false
					if err := c.d.St.SaveSettings(ctx, s); err != nil {
						return err
					}
					c.d.St.Audit(ctx, actor, "settings.remote_shell", "settings", "core", map[string]any{"enabled": false})
					return nil
				}},
		}}}, nil
}

// ---- fleet ----

// computerRows loads computers matching a WHERE clause on the computers table (alias c).
func (c *checks) computerRows(ctx context.Context, where string, args ...any) ([]model.Computer, error) {
	rows, err := c.db().QueryContext(ctx, `SELECT c.id FROM computers c WHERE `+where+` LIMIT 2000`, args...)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if len(ids) == 0 {
		return nil, rows.Err()
	}
	comps, _, err := c.d.St.ListComputers(ctx, store.ComputerFilter{IDs: ids, Limit: len(ids)})
	return comps, err
}

func computerLink(id, tab string) Remediation {
	u := "/computers/" + id
	if tab != "" {
		u += "/" + tab
	}
	return link("open", "Open computer", "", u)
}

func (c *checks) mdmUnresponsive(ctx context.Context) ([]Finding, error) {
	rows, err := c.db().QueryContext(ctx, `SELECT c.id, count(*), min(q.created_at) FROM computers c
		JOIN enrollment_queue q ON q.id = c.udid AND q.active
		LEFT JOIN command_results r ON r.command_uuid = q.command_uuid AND r.id = q.id
		WHERE c.mdm_enrolled AND c.agent_last_checkin > now() - interval '1 hour'
		  AND (r.status IS NULL OR r.status = 'NotNow') AND q.created_at < LOCALTIMESTAMP - interval '2 hours'
		  AND (c.mdm_last_checkin IS NULL OR c.mdm_last_checkin < now() - interval '2 hours')
		GROUP BY c.id`)
	if err != nil {
		return nil, err
	}
	type row struct {
		n      int
		oldest time.Time
	}
	pending := map[string]row{}
	var ids []string
	for rows.Next() {
		var id string
		var r row
		if err := rows.Scan(&id, &r.n, &r.oldest); err != nil {
			rows.Close()
			return nil, err
		}
		pending[id] = r
		ids = append(ids, id)
	}
	rows.Close()
	if len(ids) == 0 {
		return nil, nil
	}
	comps, _, err := c.d.St.ListComputers(ctx, store.ComputerFilter{IDs: ids, Limit: len(ids)})
	if err != nil {
		return nil, err
	}
	var out []Finding
	for i := range comps {
		comp := comps[i]
		p := pending[comp.ID]
		out = append(out, Finding{ObjectType: "computer", ObjectID: comp.ID, ObjectName: comp.Name, Severity: Warning,
			Summary: fmt.Sprintf("%d MDM commands waiting while the agent is online", p.n),
			Detail:  "The Mac is reachable through the agent but is not answering APNs pushes. Check that the network allows outbound TCP 5223 to Apple, or re-enroll if the push token is stale.",
			Facts:   map[string]any{"pending_commands": p.n, "oldest_queued": p.oldest, "mdm_last_checkin": comp.MDMLastCheckin, "agent_last_checkin": comp.AgentLastCheckin},
			Remediations: []Remediation{
				{ID: "push", Label: "Send an APNs push", Description: "Wakes the Mac's MDM client so it checks in and drains its queue.", Safe: true,
					Run: func(ctx context.Context) error { return c.d.Cmdr.PushComputer(ctx, &comp) }},
				computerLink(comp.ID, "commands"),
			}})
	}
	return out, nil
}

func (c *checks) agentSilent(ctx context.Context) ([]Finding, error) {
	s, err := c.d.St.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	threshold := 3 * time.Duration(s.AgentCheckinIntervalSeconds) * time.Second
	if threshold < 2*time.Hour {
		threshold = 2 * time.Hour
	}
	comps, err := c.computerRows(ctx, `c.agent_enrolled AND c.mdm_enrolled AND c.mdm_last_checkin > now() - interval '24 hours'
		AND c.agent_last_checkin < now() - $1::interval AND c.mdm_last_checkin > c.agent_last_checkin`, fmt.Sprintf("%d seconds", int(threshold.Seconds())))
	if err != nil {
		return nil, err
	}
	var out []Finding
	for _, comp := range comps {
		out = append(out, Finding{ObjectType: "computer", ObjectID: comp.ID, ObjectName: comp.Name, Severity: Warning,
			Summary:      "Agent silent while MDM is active",
			Detail:       "Run `sudo launchctl kickstart -k system/com.vibemdm.agent` on the Mac, or reinstall the agent with the install command from Settings → Enrollment.",
			Facts:        map[string]any{"agent_last_checkin": comp.AgentLastCheckin, "mdm_last_checkin": comp.MDMLastCheckin, "agent_version": comp.AgentVersion},
			Remediations: []Remediation{computerLink(comp.ID, ""), link("enroll", "Agent install command", "", "/enrollment")}})
	}
	return out, nil
}

func (c *checks) agentMissing(ctx context.Context) ([]Finding, error) {
	comps, err := c.computerRows(ctx, `c.mdm_enrolled AND NOT c.agent_enrolled AND c.mdm_enrolled_at < now() - interval '24 hours'`)
	if err != nil {
		return nil, err
	}
	var out []Finding
	for _, comp := range comps {
		out = append(out, Finding{ObjectType: "computer", ObjectID: comp.ID, ObjectName: comp.Name, Severity: Info,
			Summary:      "MDM enrolled without the agent",
			Detail:       "Policies, scripts, Self Service and extended inventory need the agent. Install it with the command from Settings → Enrollment.",
			Facts:        map[string]any{"mdm_enrolled_at": comp.MDMEnrolledAt},
			Remediations: []Remediation{link("enroll", "Agent install command", "", "/enrollment")}})
	}
	return out, nil
}

func (c *checks) diskSpace(ctx context.Context) ([]Finding, error) {
	comps, err := c.computerRows(ctx, `c.disk_free_mb IS NOT NULL AND c.disk_free_mb < 10240 AND (c.mdm_enrolled OR c.agent_enrolled)`)
	if err != nil {
		return nil, err
	}
	var out []Finding
	for _, comp := range comps {
		free := *comp.DiskFreeMB
		sev := Warning
		if free < 5120 {
			sev = Critical
		}
		out = append(out, Finding{ObjectType: "computer", ObjectID: comp.ID, ObjectName: comp.Name, Severity: sev,
			Summary:      fmt.Sprintf("%.1f GB free", float64(free)/1024),
			Detail:       "Low disk space blocks macOS updates and package installs and can prevent FileVault from completing.",
			Facts:        map[string]any{"disk_free_mb": free, "disk_total_mb": comp.DiskTotalMB, "last_user": comp.LastUser},
			Remediations: []Remediation{computerLink(comp.ID, "")}})
	}
	return out, nil
}

func (c *checks) securityPosture(ctx context.Context) ([]Finding, error) {
	comps, err := c.computerRows(ctx, `(c.sip_enabled = FALSE OR c.gatekeeper_enabled = FALSE) AND (c.mdm_enrolled OR c.agent_enrolled)`)
	if err != nil {
		return nil, err
	}
	var out []Finding
	for _, comp := range comps {
		var off []string
		if comp.SIPEnabled != nil && !*comp.SIPEnabled {
			off = append(off, "System Integrity Protection")
		}
		if comp.GatekeeperEnabled != nil && !*comp.GatekeeperEnabled {
			off = append(off, "Gatekeeper")
		}
		out = append(out, Finding{ObjectType: "computer", ObjectID: comp.ID, ObjectName: comp.Name, Severity: Warning,
			Summary:      strings.Join(off, " and ") + " disabled",
			Detail:       "SIP can only be re-enabled from Recovery (csrutil enable). Gatekeeper can be enforced with a System Policy Control profile.",
			Facts:        map[string]any{"disabled": off},
			Remediations: []Remediation{computerLink(comp.ID, "")}})
	}
	return out, nil
}

func (c *checks) fileVaultNoKey(ctx context.Context) ([]Finding, error) {
	comps, err := c.computerRows(ctx, `c.filevault_enabled AND c.mdm_enrolled
		AND NOT EXISTS (SELECT 1 FROM filevault_keys k WHERE k.computer_id = c.id)
		AND EXISTS (SELECT 1 FROM profile_deployments d JOIN config_profiles p ON p.id = d.profile_id
			WHERE d.computer_id = c.id AND p.identifier = $1 AND d.status = 'installed' AND d.updated_at < now() - interval '24 hours')`,
		escrow.EscrowProfileIdentifier)
	if err != nil {
		return nil, err
	}
	var out []Finding
	for _, comp := range comps {
		id := comp.ID
		rems := []Remediation{computerLink(id, "security")}
		if comp.AgentEnrolled {
			rems = append([]Remediation{{ID: "escrow_buddy", Label: "Generate a new key with Escrow Buddy",
				Description: "Installs Escrow Buddy; a new recovery key is generated and escrowed at the next login.", Safe: true,
				Run: func(ctx context.Context) error {
					if recent, err := c.d.St.HasRecentTask(ctx, id, "filevault_regenerate", 24*time.Hour); err != nil || recent {
						return err // already queued recently: nothing to do
					}
					if _, err := c.d.St.CreateTask(ctx, id, "filevault_regenerate", map[string]any{"reason": "health: no escrowed FileVault recovery key"}, actor); err != nil {
						return err
					}
					if c.d.Wake != nil {
						c.d.Wake(id)
					}
					return nil
				}}}, rems...)
		}
		out = append(out, Finding{ObjectType: "computer", ObjectID: comp.ID, ObjectName: comp.Name, Severity: Warning,
			Summary: "FileVault on but no recovery key escrowed",
			Detail:  "Keys are only escrowed when FileVault is enabled or the key is rotated after the escrow profile is installed. Without the agent, rotate the key manually with `fdesetup changerecovery -personal`.",
			Facts:   map[string]any{"agent_enrolled": comp.AgentEnrolled}, Remediations: rems})
	}
	return out, nil
}

func (c *checks) fileVaultNoEscrow(ctx context.Context) ([]Finding, error) {
	var n int
	err := c.db().QueryRowContext(ctx, `SELECT count(*) FROM computers c WHERE c.filevault_enabled AND c.mdm_enrolled
		AND NOT EXISTS (SELECT 1 FROM filevault_keys k WHERE k.computer_id = c.id)
		AND NOT EXISTS (SELECT 1 FROM profile_deployments d JOIN config_profiles p ON p.id = d.profile_id
			WHERE d.computer_id = c.id AND p.identifier = $1 AND d.status IN ('installed','pending'))`, escrow.EscrowProfileIdentifier).Scan(&n)
	if err != nil || n == 0 {
		return nil, err
	}
	return []Finding{{ObjectType: "server", ObjectID: "filevault_escrow", ObjectName: "FileVault escrow", Severity: Warning,
		Summary:      fmt.Sprintf("%d Mac%s with FileVault on %s not covered by the escrow profile", n, plural(n), map[bool]string{true: "is", false: "are"}[n == 1]),
		Detail:       "Deploy the FileVault escrow profile (FileVault → Profiles) and scope it to these Macs so recovery keys are collected.",
		Facts:        map[string]any{"computers": n},
		Remediations: []Remediation{link("filevault", "Configure escrow", "", "/filevault")}}}, nil
}

// ---- configuration ----

func (c *checks) profileFailures(ctx context.Context) ([]Finding, error) {
	rows, err := c.db().QueryContext(ctx, `SELECT p.id, p.name, count(*), (array_agg(d.error ORDER BY d.updated_at DESC))[1],
		array_agg(d.computer_id::text) FROM profile_deployments d JOIN config_profiles p ON p.id = d.profile_id
		JOIN computers c ON c.id = d.computer_id AND c.mdm_enrolled
		WHERE d.status = 'failed' GROUP BY p.id, p.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Finding
	for rows.Next() {
		var id, name, lastErr string
		var n int
		var idsRaw string
		if err := rows.Scan(&id, &name, &n, &lastErr, &idsRaw); err != nil {
			return nil, err
		}
		compIDs := strings.Split(strings.Trim(idsRaw, "{}"), ",")
		profileID := id
		sev := Warning
		out = append(out, Finding{ObjectType: "profile", ObjectID: id, ObjectName: name, Severity: sev,
			Summary: fmt.Sprintf("Failed to install on %d Mac%s", n, plural(n)),
			Detail:  "Most recent error: " + firstLine(lastErr),
			Facts:   map[string]any{"failed": n, "last_error": lastErr},
			Remediations: []Remediation{
				{ID: "retry", Label: "Retry on failed Macs", Description: "Re-sends InstallProfile to each Mac where it failed.", Safe: true,
					Run: func(ctx context.Context) error {
						prof, err := c.d.St.GetProfile(ctx, profileID)
						if err != nil {
							return err
						}
						failed, total := 0, 0
						var firstErr error
						for _, cid := range compIDs {
							comp, err := c.d.St.GetComputer(ctx, cid)
							if err != nil {
								continue
							}
							total++
							if err := c.d.Cmdr.InstallProfile(ctx, comp, prof, actor); err != nil {
								failed++
								if firstErr == nil {
									firstErr = err
								}
							}
						}
						if failed > 0 {
							return fmt.Errorf("could not queue on %d of %d Macs: %w", failed, total, firstErr)
						}
						return nil
					}},
				link("open", "Open profile", "", "/profiles/"+id),
			}})
	}
	return out, rows.Err()
}

// declarationIssues walks a DDM status tree for declarations reported invalid.
func declarationIssues(v any, out map[string]string) {
	switch t := v.(type) {
	case map[string]any:
		if id, ok := t["identifier"].(string); ok {
			valid := fmt.Sprint(t["valid"])
			if valid == "invalid" || valid == "false" {
				reason := ""
				if rs, ok := t["reasons"].([]any); ok && len(rs) > 0 {
					if r, ok := rs[0].(map[string]any); ok {
						reason = fmt.Sprint(r["description"])
						if reason == "" || reason == "<nil>" {
							reason = fmt.Sprint(r["code"])
						}
					}
				}
				out[id] = reason
			}
			return
		}
		for _, child := range t {
			declarationIssues(child, out)
		}
	case []any:
		for _, child := range t {
			declarationIssues(child, out)
		}
	}
}

func (c *checks) ddmInvalid(ctx context.Context) ([]Finding, error) {
	rows, err := c.db().QueryContext(ctx, `SELECT s.computer_id, c.name, s.declarations_status, s.status->'management' FROM ddm_device_state s
		JOIN computers c ON c.id = s.computer_id AND c.mdm_enrolled WHERE s.last_status_at IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Finding
	for rows.Next() {
		var id, name string
		var declRaw, mgmtRaw []byte
		if err := rows.Scan(&id, &name, &declRaw, &mgmtRaw); err != nil {
			return nil, err
		}
		issues := map[string]string{}
		for _, raw := range [][]byte{declRaw, mgmtRaw} {
			var v any
			if len(raw) > 0 && json.Unmarshal(raw, &v) == nil {
				declarationIssues(v, issues)
			}
		}
		if len(issues) == 0 {
			continue
		}
		var names []string
		for k, r := range issues {
			if r != "" {
				names = append(names, k+" ("+r+")")
			} else {
				names = append(names, k)
			}
		}
		out = append(out, Finding{ObjectType: "computer", ObjectID: id, ObjectName: name, Severity: Warning,
			Summary:      fmt.Sprintf("%d declaration%s invalid", len(issues), plural(len(issues))),
			Detail:       "The Mac rejected: " + strings.Join(names, "; ") + ". Check the macOS version requirement and whether the declaration needs supervision.",
			Facts:        map[string]any{"declarations": issues},
			Remediations: []Remediation{computerLink(id, "updates"), link("plans", "Review update plans", "", "/updates/plans")}})
	}
	return out, rows.Err()
}

func (c *checks) ddmStuck(ctx context.Context) ([]Finding, error) {
	comps, err := c.computerRows(ctx, `c.mdm_enrolled AND EXISTS (SELECT 1 FROM ddm_device_state s WHERE s.computer_id = c.id
		AND s.sync_sent_token <> '' AND s.sync_sent_token <> s.served_token AND s.sync_sent_at < now() - interval '6 hours')`)
	if err != nil {
		return nil, err
	}
	var out []Finding
	for i := range comps {
		comp := comps[i]
		out = append(out, Finding{ObjectType: "computer", ObjectID: comp.ID, ObjectName: comp.Name, Severity: Info,
			Summary: "Declarations not fetched for over 6 hours",
			Detail:  "The Mac was asked to sync its declarations but has not fetched them. It may be offline, or MDM pushes are not reaching it.",
			Facts:   map[string]any{"mdm_last_checkin": comp.MDMLastCheckin},
			Remediations: []Remediation{
				{ID: "resync", Label: "Ask the Mac to sync declarations", Description: "Sends a DeclarativeManagement command with the current token.", Safe: true,
					Run: func(ctx context.Context) error { return c.d.Cmdr.SyncDDM(ctx, &comp, actor) }},
				computerLink(comp.ID, "updates"),
			}})
	}
	return out, nil
}

// runFailures evaluates the latest run per (object, computer) over 7 days.
func (c *checks) runFailures(ctx context.Context, q, objectType, urlPrefix, noun string) ([]Finding, error) {
	rows, err := c.db().QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Finding
	for rows.Next() {
		var id, name, lastLog string
		var failed, total int
		if err := rows.Scan(&id, &name, &failed, &total, &lastLog); err != nil {
			return nil, err
		}
		if failed < 3 || failed*2 < total {
			continue
		}
		out = append(out, Finding{ObjectType: objectType, ObjectID: id, ObjectName: name, Severity: Warning,
			Summary:      fmt.Sprintf("Latest %s failed on %d of %d Macs", noun, failed, total),
			Detail:       "Last failure output: " + firstLine(tail(lastLog, 400)),
			Facts:        map[string]any{"failed": failed, "total": total},
			Remediations: []Remediation{link("logs", "Review logs", "", urlPrefix+id)}})
	}
	return out, rows.Err()
}

func (c *checks) policyFailures(ctx context.Context) ([]Finding, error) {
	return c.runFailures(ctx, `SELECT p.id, p.name, count(*) FILTER (WHERE l.status = 'failed'), count(*),
		COALESCE((array_agg(l.log ORDER BY l.created_at DESC) FILTER (WHERE l.status = 'failed'))[1], '')
		FROM policies p JOIN LATERAL (SELECT DISTINCT ON (computer_id) status, log, created_at FROM policy_logs
			WHERE policy_id = p.id AND created_at > now() - interval '7 days' AND status IN ('completed','failed')
			ORDER BY computer_id, created_at DESC) l ON TRUE
		WHERE p.enabled GROUP BY p.id, p.name`, "policy", "/policies/", "run")
}

func (c *checks) appFailures(ctx context.Context) ([]Finding, error) {
	return c.runFailures(ctx, `SELECT a.id, a.name, count(*) FILTER (WHERE l.status = 'failed'), count(*),
		COALESCE((array_agg(l.log ORDER BY l.created_at DESC) FILTER (WHERE l.status = 'failed'))[1], '')
		FROM mac_apps a JOIN LATERAL (SELECT DISTINCT ON (computer_id) status, log, created_at FROM app_install_logs
			WHERE app_id = a.id AND created_at > now() - interval '7 days' AND status IN ('completed','failed')
			ORDER BY computer_id, created_at DESC) l ON TRUE
		WHERE a.enabled GROUP BY a.id, a.name`, "app", "/apps/", "install")
}

func (c *checks) policyNeverRuns(ctx context.Context) ([]Finding, error) {
	pols, err := c.d.St.ListPolicies(ctx)
	if err != nil {
		return nil, err
	}
	var out []Finding
	for _, p := range pols {
		if !p.Enabled {
			continue
		}
		t := p.Triggers
		noTrigger := !t.Startup && !t.Login && !t.Checkin && !t.EnrollmentComplete && !t.NetworkChange && strings.TrimSpace(t.Custom) == "" && !p.SelfService.Enabled
		empty := len(p.Scripts) == 0 && len(p.Packages) == 0 && len(p.Apps) == 0 && strings.TrimSpace(p.RunCommand) == "" && !p.UpdateInventory
		var why string
		switch {
		case noTrigger:
			why = "It has no trigger and is not available in Self Service, so it will never run."
		case empty:
			why = "It has no scripts, packages, apps, command or inventory step, so running it does nothing."
		default:
			continue
		}
		out = append(out, Finding{ObjectType: "policy", ObjectID: p.ID, ObjectName: p.Name, Severity: Info,
			Summary: "Enabled policy has no effect", Detail: why,
			Remediations: []Remediation{link("open", "Edit policy", "", "/policies/"+p.ID)}})
	}
	return out, nil
}

func (c *checks) staleScope(ctx context.Context) ([]Finding, error) {
	exists := func(table string) (map[string]bool, error) {
		rows, err := c.db().QueryContext(ctx, `SELECT id::text FROM `+table)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		m := map[string]bool{}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				return nil, err
			}
			m[id] = true
		}
		return m, rows.Err()
	}
	groups, err := exists("computer_groups")
	if err != nil {
		return nil, err
	}
	comps, err := exists("computers")
	if err != nil {
		return nil, err
	}
	segs, err := exists("network_segments")
	if err != nil {
		return nil, err
	}
	missing := func(sc model.Scope) []string {
		var out []string
		for _, sel := range []model.ScopeSelection{sc.Targets, sc.Exclusions} {
			for _, id := range sel.GroupIDs {
				if !groups[id] {
					out = append(out, "group "+id)
				}
			}
			for _, id := range sel.ComputerIDs {
				if !comps[id] {
					out = append(out, "computer "+id)
				}
			}
			for _, id := range sel.NetworkSegments {
				if !segs[id] {
					out = append(out, "network segment "+id)
				}
			}
		}
		for _, id := range sc.Limitations.NetworkSegments {
			if !segs[id] {
				out = append(out, "network segment "+id)
			}
		}
		return out
	}
	var out []Finding
	add := func(objType, id, name, url string, sc model.Scope) {
		if m := missing(sc); len(m) > 0 {
			out = append(out, Finding{ObjectType: objType, ObjectID: id, ObjectName: name, Severity: Warning,
				Summary: fmt.Sprintf("Scope references %d deleted object%s", len(m), plural(len(m))),
				Detail:  "Missing: " + strings.Join(m, ", ") + ". Targets that no longer exist silently match nothing; deleted exclusions no longer exclude anything.",
				Facts:   map[string]any{"missing": m}, Remediations: []Remediation{link("open", "Edit scope", "", url)}})
		}
	}
	pols, err := c.d.St.ListPolicies(ctx)
	if err != nil {
		return nil, err
	}
	for _, p := range pols {
		add("policy", p.ID, p.Name, "/policies/"+p.ID, p.Scope)
	}
	profs, err := c.d.St.ListProfiles(ctx)
	if err != nil {
		return nil, err
	}
	for _, p := range profs {
		add("profile", p.ID, p.Name, "/profiles/"+p.ID, p.Scope)
	}
	apps, err := c.d.St.ListMacApps(ctx)
	if err != nil {
		return nil, err
	}
	for _, a := range apps {
		add("app", a.ID, a.Name, "/apps/"+a.ID, a.Scope)
	}
	plans, err := c.d.St.ListUpdatePlans(ctx)
	if err != nil {
		return nil, err
	}
	for _, p := range plans {
		add("plan", p.ID, p.Name, "/updates/plans/"+p.ID, p.Scope)
	}
	return out, nil
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func tail(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	// Prefer the last meaningful line of long output.
	lines := strings.Split(s, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if l := strings.TrimSpace(lines[i]); l != "" {
			return l
		}
	}
	return s[len(s)-n:]
}
