// Package mdmsvc wraps the NanoMDM core service to capture inventory and
// drive VibeMDM behaviour from MDM protocol events.
package mdmsvc

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/micromdm/nanomdm/mdm"
	"github.com/micromdm/nanomdm/service"
	"howett.net/plist"

	"github.com/vibemdm/vibemdm/internal/escrow"
	"github.com/vibemdm/vibemdm/internal/model"
	"github.com/vibemdm/vibemdm/internal/store"
	"github.com/vibemdm/vibemdm/internal/webhook"
)

// Service implements service.CheckinAndCommandService.
type Service struct {
	next   service.CheckinAndCommandService
	st     *store.Store
	cmdr   *Commander
	hooks  *webhook.Dispatcher
	escrow *escrow.Identity
	log    *slog.Logger
}

// WithEscrow enables FileVault PRK decryption from SecurityInfo responses.
func (s *Service) WithEscrow(id *escrow.Identity) *Service {
	s.escrow = id
	return s
}

// New wraps next.
func New(st *store.Store, next service.CheckinAndCommandService, cmdr *Commander, hooks *webhook.Dispatcher, logger *slog.Logger) *Service {
	return &Service{next: next, st: st, cmdr: cmdr, hooks: hooks, log: logger}
}

type authenticateMsg struct {
	UDID         string
	SerialNumber string
	DeviceName   string
	Model        string
	ModelName    string
	OSVersion    string
	BuildVersion string
	ProductName  string
}

// Authenticate handles the first enrollment message.
func (s *Service) Authenticate(r *mdm.Request, m *mdm.Authenticate) error {
	if err := s.next.Authenticate(r, m); err != nil {
		return err
	}
	if m.UDID == "" {
		return nil
	}
	var a authenticateMsg
	if _, err := plist.Unmarshal(m.Raw, &a); err != nil {
		s.log.Warn("parsing Authenticate", "err", err)
	}
	if a.UDID == "" {
		a.UDID = m.UDID
	}
	if a.SerialNumber == "" {
		a.SerialNumber = m.SerialNumber
	}
	_, err := s.st.UpsertComputerFromAuthenticate(r.Context(), store.MDMAuthenticateInfo{
		UDID: a.UDID, SerialNumber: a.SerialNumber, DeviceName: a.DeviceName, Model: a.Model, ModelName: a.ModelName,
		OSVersion: a.OSVersion, BuildVersion: a.BuildVersion, ProductName: a.ProductName,
	})
	if err != nil {
		s.log.Error("upsert computer from Authenticate", "err", err, "udid", a.UDID)
	} else {
		s.log.Info("mdm: device authenticated", "udid", a.UDID, "serial", a.SerialNumber, "name", a.DeviceName)
	}
	return nil
}

// TokenUpdate completes (or refreshes) an enrollment.
func (s *Service) TokenUpdate(r *mdm.Request, m *mdm.TokenUpdate) error {
	if err := s.next.TokenUpdate(r, m); err != nil {
		return err
	}
	if m.UDID == "" || m.UserID != "" || m.EnrollmentUserID != "" {
		return nil // user channel
	}
	comp, wasEnrolled, err := s.st.MarkMDMEnrolled(r.Context(), m.UDID)
	if err != nil {
		s.log.Error("mark MDM enrolled", "err", err, "udid", m.UDID)
		return nil
	}
	s.log.Info("mdm: token update", "udid", m.UDID, "computer", comp.Name, "new_enrollment", !wasEnrolled)
	if !wasEnrolled {
		s.hooks.Fire(webhook.ComputerEnrolled, map[string]any{"computer_id": comp.ID, "computer": comp.Name, "serial": comp.SerialNumber, "udid": comp.UDID, "os_version": comp.OSVersion})
	}
	if !wasEnrolled && s.cmdr != nil {
		go func(c model.Computer) {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			// Give the device a moment to settle, then queue inventory and scoped profiles.
			time.Sleep(2 * time.Second)
			if err := s.cmdr.RequestInventory(ctx, &c, "system"); err != nil {
				s.log.Warn("initial inventory request", "err", err)
			}
			if err := s.cmdr.SyncProfilesForComputer(ctx, &c, "system"); err != nil {
				s.log.Warn("initial profile sync", "err", err)
			}
		}(*comp)
	}
	return nil
}

// CheckOut handles unenrollment.
func (s *Service) CheckOut(r *mdm.Request, m *mdm.CheckOut) error {
	err := s.next.CheckOut(r, m)
	if m.UDID != "" && m.UserID == "" {
		if e := s.st.MarkMDMUnenrolled(r.Context(), m.UDID); e != nil {
			s.log.Error("mark unenrolled", "err", e)
		} else {
			s.log.Info("mdm: device checked out", "udid", m.UDID)
			if comp, cerr := s.st.GetComputerByUDID(r.Context(), m.UDID); cerr == nil {
				s.hooks.Fire(webhook.ComputerUnenrolled, map[string]any{"computer_id": comp.ID, "computer": comp.Name, "serial": comp.SerialNumber, "udid": m.UDID})
			}
		}
	}
	return err
}

// SetBootstrapToken passes through and records escrow.
func (s *Service) SetBootstrapToken(r *mdm.Request, m *mdm.SetBootstrapToken) error {
	err := s.next.SetBootstrapToken(r, m)
	if err == nil && m.UDID != "" {
		s.st.DB.ExecContext(r.Context(), `UPDATE computers SET bootstrap_token_escrowed = TRUE, updated_at = now() WHERE udid = $1`, m.UDID)
	}
	return err
}

// GetBootstrapToken passes through.
func (s *Service) GetBootstrapToken(r *mdm.Request, m *mdm.GetBootstrapToken) (*mdm.BootstrapToken, error) {
	return s.next.GetBootstrapToken(r, m)
}

// UserAuthenticate passes through.
func (s *Service) UserAuthenticate(r *mdm.Request, m *mdm.UserAuthenticate) ([]byte, error) {
	return s.next.UserAuthenticate(r, m)
}

// DeclarativeManagement passes through.
func (s *Service) DeclarativeManagement(r *mdm.Request, m *mdm.DeclarativeManagement) ([]byte, error) {
	return s.next.DeclarativeManagement(r, m)
}

// GetToken passes through.
func (s *Service) GetToken(r *mdm.Request, m *mdm.GetToken) (*mdm.GetTokenResponse, error) {
	return s.next.GetToken(r, m)
}

// CommandAndReportResults records results into inventory.
func (s *Service) CommandAndReportResults(r *mdm.Request, results *mdm.CommandResults) (*mdm.Command, error) {
	cmd, err := s.next.CommandAndReportResults(r, results)
	if results.UDID != "" && results.UserID == "" && results.EnrollmentUserID == "" {
		s.st.TouchMDMCheckin(r.Context(), results.UDID)
		if results.CommandUUID != "" && results.Status != "Idle" && results.Status != "NotNow" {
			s.processResult(r.Context(), results)
		}
	}
	return cmd, err
}

func (s *Service) processResult(ctx context.Context, results *mdm.CommandResults) {
	meta, err := s.st.GetCommandMeta(ctx, results.CommandUUID)
	requestType := ""
	if err == nil {
		requestType = meta.RequestType
	} else {
		// Fall back to the NanoMDM commands table.
		s.st.DB.QueryRowContext(ctx, `SELECT request_type FROM commands WHERE command_uuid = $1`, results.CommandUUID).Scan(&requestType)
	}
	var doc map[string]any
	if _, err := plist.Unmarshal(results.Raw, &doc); err != nil {
		s.log.Warn("parsing command result", "err", err, "uuid", results.CommandUUID)
		return
	}
	errText := ""
	if results.Status != "Acknowledged" {
		var parts []string
		for _, e := range results.ErrorChain {
			if e.LocalizedDescription != "" {
				parts = append(parts, e.LocalizedDescription)
			} else if e.USEnglishDescription != "" {
				parts = append(parts, e.USEnglishDescription)
			}
		}
		errText = results.Status
		if len(parts) > 0 {
			errText += ": " + strings.Join(parts, "; ")
		}
	}
	comp, compErr := s.st.GetComputerByUDID(ctx, results.UDID)
	if compErr == nil && requestType != "" {
		s.hooks.Fire(webhook.MDMCommandCompleted, map[string]any{"computer_id": comp.ID, "computer": comp.Name, "command_uuid": results.CommandUUID,
			"request_type": requestType, "status": results.Status, "error": errText})
	}
	switch requestType {
	case "DeviceInformation":
		if q, ok := doc["QueryResponses"].(map[string]any); ok {
			if err := s.st.ApplyMDMDeviceInformation(ctx, results.UDID, q); err != nil {
				s.log.Error("apply DeviceInformation", "err", err)
			}
		}
	case "SecurityInfo":
		if sec, ok := doc["SecurityInfo"].(map[string]any); ok {
			// The escrowed recovery key arrives CMS-encrypted to our escrow
			// certificate. Never store the blob in inventory JSON.
			cms, hasKey := sec["FDE_PersonalRecoveryKeyCMS"].([]byte)
			delete(sec, "FDE_PersonalRecoveryKeyCMS")
			if err := s.st.ApplyMDMSecurityInfo(ctx, results.UDID, sec); err != nil {
				s.log.Error("apply SecurityInfo", "err", err)
			}
			if hasKey && compErr == nil {
				s.escrowKey(ctx, comp, cms)
			}
		}
	case "ProfileList":
		if list, ok := doc["ProfileList"].([]any); ok && compErr == nil {
			profiles := make([]model.ComputerProfile, 0, len(list))
			for _, it := range list {
				m, _ := it.(map[string]any)
				if m == nil {
					continue
				}
				profiles = append(profiles, model.ComputerProfile{
					Identifier: str(m["PayloadIdentifier"]), UUID: str(m["PayloadUUID"]), DisplayName: str(m["PayloadDisplayName"]),
					Organization: str(m["PayloadOrganization"]), IsManaged: boolean(m["IsManaged"]), Source: "mdm",
				})
			}
			if err := s.st.ReplaceComputerProfiles(ctx, comp.ID, "mdm", profiles); err != nil {
				s.log.Error("replace profiles", "err", err)
			}
			// Agent-reported profiles would duplicate these.
			s.st.DB.ExecContext(ctx, `DELETE FROM computer_profiles WHERE computer_id = $1 AND source = 'agent'`, comp.ID)
			s.reconcileDeployments(ctx, comp, profiles)
		}
	case "InstalledApplicationList":
		if list, ok := doc["InstalledApplicationList"].([]any); ok && compErr == nil && !comp.AgentEnrolled {
			apps := make([]model.ComputerApp, 0, len(list))
			for _, it := range list {
				m, _ := it.(map[string]any)
				if m == nil {
					continue
				}
				ver := str(m["ShortVersion"])
				if ver == "" {
					ver = str(m["Version"])
				}
				apps = append(apps, model.ComputerApp{Name: str(m["Name"]), Version: ver, BundleID: str(m["Identifier"]), Source: "mdm"})
			}
			if err := s.st.ReplaceComputerApps(ctx, comp.ID, "mdm", apps); err != nil {
				s.log.Error("replace apps", "err", err)
			}
		}
	case "InstallProfile":
		status := "installed"
		if errText != "" {
			status = "failed"
		}
		s.st.SetDeploymentStatusByCommand(ctx, results.CommandUUID, status, errText)
	case "RemoveProfile":
		status := "removed"
		if errText != "" {
			status = "failed"
		}
		s.st.SetDeploymentStatusByCommand(ctx, results.CommandUUID, status, errText)
	}
	if compErr == nil && requestType != "" {
		s.log.Debug("mdm: command result", "type", requestType, "status", results.Status, "computer", comp.Name)
		s.hooks.Fire(webhook.MDMCommandCompleted, map[string]any{"computer_id": comp.ID, "computer": comp.Name, "request_type": requestType,
			"command_uuid": results.CommandUUID, "status": results.Status})
	}
}

// escrowKey decrypts and stores a FileVault personal recovery key.
func (s *Service) escrowKey(ctx context.Context, comp *model.Computer, cms []byte) {
	if s.escrow == nil {
		s.log.Warn("received an escrowed FileVault key but no escrow identity is loaded", "computer", comp.Name)
		return
	}
	prk, err := s.escrow.DecryptPRK(cms)
	if err != nil {
		s.log.Error("decrypting escrowed FileVault key", "computer", comp.Name, "err", err)
		return
	}
	changed, err := s.st.SaveFileVaultKey(ctx, comp.ID, prk, "mdm")
	if err != nil {
		s.log.Error("storing FileVault key", "computer", comp.Name, "err", err)
		return
	}
	if changed {
		s.log.Info("FileVault recovery key escrowed", "computer", comp.Name, "fingerprint", escrow.Fingerprint(prk))
		s.st.Audit(ctx, "system", "filevault.key_escrowed", "computer", comp.ID, map[string]any{"fingerprint": escrow.Fingerprint(prk)})
		s.hooks.Fire(webhook.FileVaultKeyEscrowed, map[string]any{"computer_id": comp.ID, "computer": comp.Name, "serial": comp.SerialNumber, "fingerprint": escrow.Fingerprint(prk)})
	}
}

// reconcileDeployments marks deployments installed when the device reports the profile present.
func (s *Service) reconcileDeployments(ctx context.Context, comp *model.Computer, installed []model.ComputerProfile) {
	deps, err := s.st.ListDeployments(ctx, "", comp.ID)
	if err != nil {
		return
	}
	present := map[string]bool{}
	for _, p := range installed {
		present[p.Identifier] = true
	}
	for _, d := range deps {
		prof, err := s.st.GetProfile(ctx, d.ProfileID)
		if err != nil {
			continue
		}
		if present[prof.Identifier] && d.Status == "pending" {
			s.st.UpsertDeployment(ctx, d.ProfileID, comp.ID, "installed", d.CommandUUID, "")
		}
	}
}

func str(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func boolean(v any) bool {
	b, _ := v.(bool)
	return b
}
