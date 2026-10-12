package agentinstall

import (
	"context"
	"crypto/x509"
	_ "embed"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"howett.net/plist"

	"github.com/vibemdm/vibemdm/internal/auth"
	"github.com/vibemdm/vibemdm/internal/commands"
	"github.com/vibemdm/vibemdm/internal/model"
	"github.com/vibemdm/vibemdm/internal/store"
)

// Building the package: VibeMDM builds and uploads the agent package itself on an enrolled Mac
// (POST /api/v1/agent-package/build {"computer_id": …}), with no Terminal step:
//
//  1. a temporary profile makes the Mac trust the package-signing CA (productsign only uses a
//     certificate whose CA the Mac trusts, and only a profile can add trust without a person
//     approving it);
//  2. the agent runs build-agent-pkg.sh (embedded below) with a one-hour API token;
//  3. the profile and the token are removed again, whatever happened.

//go:embed build-agent-pkg.sh
var buildScript string

// CATrustIdentifier is the PayloadIdentifier of the temporary CA-trust profile.
const CATrustIdentifier = "com.vibemdm.agent-package.ca-trust"

// BuildState reports the last or current "Build on a Mac" run.
type BuildState struct {
	Status       string     `json:"status"` // idle | trusting | building | cleaning | completed | failed
	ComputerID   string     `json:"computer_id,omitempty"`
	ComputerName string     `json:"computer_name,omitempty"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
	Message      string     `json:"message,omitempty"`
	Output       string     `json:"output,omitempty"` // the build script's output (last 4 KB)
}

type buildRun struct {
	mu    sync.Mutex
	state BuildState
}

// ErrBuildRunning means a build is already in progress.
var ErrBuildRunning = errors.New("a package build is already running")

// BuildState returns the state of the last build.
func (s *Service) BuildState() BuildState {
	s.build.mu.Lock()
	defer s.build.mu.Unlock()
	if s.build.state.Status == "" {
		return BuildState{Status: "idle"}
	}
	return s.build.state
}

func (s *Service) setBuild(f func(*BuildState)) {
	s.build.mu.Lock()
	defer s.build.mu.Unlock()
	f(&s.build.state)
}

// Build starts building the agent package on comp (asynchronously; see BuildState).
func (s *Service) Build(ctx context.Context, comp *model.Computer, by string) error {
	if !comp.MDMEnrolled || !comp.AgentEnrolled {
		return errors.New("choose a Mac that is enrolled in MDM and has the agent")
	}
	settings, err := s.st.GetSettings(ctx)
	if err != nil {
		return err
	}
	if settings.ServerURL == "" {
		return errors.New("the server URL is not configured (Settings → General)")
	}
	s.build.mu.Lock()
	switch s.build.state.Status {
	case "trusting", "building", "cleaning":
		s.build.mu.Unlock()
		return ErrBuildRunning
	}
	now := time.Now().UTC()
	s.build.state = BuildState{Status: "trusting", ComputerID: comp.ID, ComputerName: comp.Name, StartedAt: &now,
		Message: "Making " + comp.Name + " trust the package-signing CA"}
	s.build.mu.Unlock()
	s.st.Audit(ctx, by, "agent_package.build", "computer", comp.ID, map[string]any{"computer": comp.Name})
	go s.runBuild(*comp, strings.TrimRight(settings.ServerURL, "/"), settings.OrgName, by)
	return nil
}

func (s *Service) runBuild(comp model.Computer, serverURL, org, by string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	fail := func(msg string) {
		now := time.Now().UTC()
		s.setBuild(func(b *BuildState) { b.Status, b.Message, b.FinishedAt = "failed", msg, &now })
		s.log.Warn("agent package build failed", "computer", comp.Name, "err", msg)
	}
	caPEM, _, err := s.CA(ctx)
	if err != nil {
		fail("package-signing CA: " + err.Error())
		return
	}
	prof, err := CATrustProfile(org, caPEM)
	if err != nil {
		fail(err.Error())
		return
	}
	pc, err := commands.InstallProfile(prof)
	if err != nil {
		fail(err.Error())
		return
	}
	// Whatever happens next, the trust is taken away again.
	defer func() {
		if rc, err := commands.RemoveProfile(CATrustIdentifier); err == nil {
			if _, err := s.cmdr.Enqueue(context.Background(), &comp, rc, "agent_pkg_build", "", by); err != nil {
				s.log.Warn("removing the temporary CA-trust profile", "computer", comp.Name, "err", err)
			}
		}
	}()
	if _, err := s.cmdr.Enqueue(ctx, &comp, pc, "agent_pkg_build", "", by); err != nil {
		fail("sending the CA-trust profile: " + err.Error())
		return
	}
	if err := s.waitCommand(ctx, pc.CommandUUID, 10*time.Minute); err != nil {
		fail("the Mac did not install the CA-trust profile: " + err.Error())
		return
	}

	// A one-hour admin token for the script's two API calls, deleted afterwards.
	tok, prefix := auth.NewAPIToken()
	exp := time.Now().Add(time.Hour)
	t, err := s.st.CreateAPIToken(ctx, "Agent package build on "+comp.Name+" (temporary)", store.HashToken(tok), prefix, "admin", "", &exp)
	if err != nil {
		fail("creating a temporary API token: " + err.Error())
		return
	}
	defer s.st.DeleteAPIToken(context.Background(), t.ID)

	s.setBuild(func(b *BuildState) {
		b.Status, b.Message = "building", "Building and signing the package on "+comp.Name
	})
	before, _ := s.Package(ctx)
	task, err := s.st.CreateTask(ctx, comp.ID, "run_command", map[string]any{"command": buildCommand(serverURL, tok)}, by)
	if err != nil {
		fail("queueing the build on the Mac: " + err.Error())
		return
	}
	if s.Wake != nil {
		s.Wake(comp.ID)
	}
	res, err := s.waitTask(ctx, task.ID, 20*time.Minute)
	out := ""
	if res != nil {
		out = strings.ReplaceAll(firstString(res.Result, "stdout", "combined")+firstString(res.Result, "stderr"), tok, "[token]")
		if len(out) > 4096 {
			out = "…" + out[len(out)-4096:]
		}
	}
	s.setBuild(func(b *BuildState) {
		b.Status, b.Output, b.Message = "cleaning", out, "Removing the temporary trust and token"
	})
	if err != nil {
		fail(err.Error())
		return
	}
	after, perr := s.Package(ctx)
	if res.Status != "completed" || perr != nil || (before != nil && !after.UploadedAt.After(before.UploadedAt)) {
		fail("the build did not upload a package; see the output")
		return
	}
	now := time.Now().UTC()
	s.setBuild(func(b *BuildState) {
		b.Status, b.FinishedAt, b.Message = "completed", &now, "Package "+after.Version+" built on "+comp.Name+" and uploaded"
	})
	s.log.Info("agent package built", "computer", comp.Name, "version", after.Version)
}

// buildCommand runs the embedded build script as root on the Mac.
func buildCommand(serverURL, token string) string {
	return `umask 022
S=$(mktemp /tmp/vibemdm-build.XXXXXX) || exit 1
chmod 700 "$S"
echo '` + base64.StdEncoding.EncodeToString([]byte(buildScript)) + `' | /usr/bin/base64 -D > "$S"
VIBEMDM_URL='` + strings.ReplaceAll(serverURL, "'", "") + `' VIBEMDM_API_TOKEN='` + token + `' /bin/bash "$S"
rc=$?
rm -f "$S"
exit $rc`
}

func firstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

// waitCommand waits until the Mac answers an MDM command.
func (s *Service) waitCommand(ctx context.Context, uuid string, max time.Duration) error {
	deadline := time.Now().Add(max)
	for time.Now().Before(deadline) {
		st, err := s.st.MDMCommandStatus(ctx, uuid)
		if err != nil {
			return err
		}
		switch st {
		case "Acknowledged":
			return nil
		case "":
		default:
			return fmt.Errorf("the Mac answered %s", st)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
	return errors.New("no answer from the Mac (is it online?)")
}

// waitTask waits for an agent task to finish.
func (s *Service) waitTask(ctx context.Context, id string, max time.Duration) (*model.AgentTask, error) {
	deadline := time.Now().Add(max)
	for time.Now().Before(deadline) {
		t, err := s.st.GetTask(ctx, id)
		if err != nil {
			return nil, err
		}
		if t.Status == "completed" || t.Status == "failed" {
			return t, nil
		}
		select {
		case <-ctx.Done():
			return t, ctx.Err()
		case <-time.After(4 * time.Second):
		}
	}
	return nil, errors.New("the build did not finish in time (is the agent online?)")
}

// CATrustProfile makes a Mac trust the package-signing CA (com.apple.security.root).
func CATrustProfile(org, caPEM string) ([]byte, error) {
	b, _ := pem.Decode([]byte(caPEM))
	if b == nil {
		return nil, errors.New("package-signing CA is unreadable")
	}
	if _, err := x509.ParseCertificate(b.Bytes); err != nil {
		return nil, err
	}
	if org == "" {
		org = "VibeMDM"
	}
	p := map[string]any{
		"PayloadType": "Configuration", "PayloadVersion": 1, "PayloadIdentifier": CATrustIdentifier,
		"PayloadUUID": stableUUID(CATrustIdentifier), "PayloadDisplayName": org + " package signing (temporary)",
		"PayloadDescription":  "Lets this Mac sign the VibeMDM agent package. Removed automatically when the build finishes.",
		"PayloadOrganization": org, "PayloadScope": "System",
		"PayloadContent": []any{map[string]any{
			"PayloadType": "com.apple.security.root", "PayloadVersion": 1, "PayloadIdentifier": CATrustIdentifier + ".root",
			"PayloadUUID": stableUUID(CATrustIdentifier + ".root"), "PayloadDisplayName": org + " Package Signing CA",
			"PayloadContent": b.Bytes,
		}},
	}
	return plist.MarshalIndent(p, plist.XMLFormat, "\t")
}
