// Package superwf deploys S.U.P.E.R.M.A.N. ("super", by Kevin M. White,
// github.com/Macjutsu/super, Apache-2.0) natively: VibeMDM installs a pinned,
// checksum-verified super on in-scope Macs through the agent, configures it
// with a managed-preferences profile, and reads its status into inventory.
package superwf

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/vibemdm/vibemdm/internal/model"
	"github.com/vibemdm/vibemdm/internal/store"
)

// Pinned super release. super 6.x (pkg-only, Apple silicon, macOS 15+) is in
// beta; VibeMDM ships 5.1.x until 6.0 is released.
const (
	Version   = "5.1.2"
	ScriptURL = "https://raw.githubusercontent.com/Macjutsu/super/v5.1.2/super"
	SHA256    = "a8ac2a725c7b1402b23e835dc507eb9a37a3d67c20d25abab31b4f6ab221c2d6"
	// ProfileIdentifier is the generated preferences profile.
	ProfileIdentifier = "com.vibemdm.super"
	SchemaRef         = "builtin:com.macjutsu.super"
)

// Service reconciles super installations.
type Service struct {
	st     *store.Store
	notify func(string)
	log    *slog.Logger
}

// New creates the service.
func New(st *store.Store, notify func(string), logger *slog.Logger) *Service {
	return &Service{st: st, notify: notify, log: logger}
}

// InstalledVersion extracts the super version a Mac reports ("" if none).
func InstalledVersion(sup map[string]any) string {
	if sup == nil {
		return ""
	}
	if v, ok := sup["SuperVersion"].(string); ok && v != "" {
		return v
	}
	return "unknown"
}

// QueueInstall sends an install_super task to one computer.
func (s *Service) QueueInstall(ctx context.Context, comp *model.Computer, by string) error {
	if !comp.AgentEnrolled {
		return fmt.Errorf("%s does not have the VibeMDM agent", comp.Name)
	}
	cfg, pw, err := s.st.GetSuperConfig(ctx)
	if err != nil {
		return err
	}
	payload := map[string]any{"url": ScriptURL, "sha256": SHA256, "version": Version, "auth_mode": cfg.AuthMode, "auth_account": cfg.AuthAccount}
	if cfg.AuthMode != "none" && pw != "" {
		payload["auth_password"] = pw // scrubbed from the task when it ends
	}
	if _, err := s.st.CreateTask(ctx, comp.ID, "install_super", payload, by); err != nil {
		return err
	}
	if s.notify != nil {
		s.notify(comp.ID)
	}
	return nil
}

// Reconcile installs (or upgrades) super on in-scope Macs that lack the pinned
// version, at most once a day per Mac. It returns how many installs it queued.
func (s *Service) Reconcile(ctx context.Context) int {
	cfg, _, err := s.st.GetSuperConfig(ctx)
	if err != nil || !cfg.Enabled {
		return 0
	}
	ids, err := s.st.ResolveScope(ctx, cfg.Scope)
	if err != nil {
		s.log.Warn("super: resolving scope", "err", err)
		return 0
	}
	rows, err := s.st.SuperStatus(ctx, ids)
	if err != nil {
		return 0
	}
	queued := 0
	for _, r := range rows {
		if InstalledVersion(r.Super) == Version {
			continue
		}
		comp, err := s.st.GetComputer(ctx, r.ComputerID)
		if err != nil || !comp.AgentEnrolled {
			continue
		}
		if recent, err := s.st.HasRecentTask(ctx, comp.ID, "install_super", 24*time.Hour); err != nil || recent {
			continue
		}
		if err := s.QueueInstall(ctx, comp, "system"); err == nil {
			queued++
		}
	}
	if queued > 0 {
		s.log.Info("queued super installs", "count", queued, "version", Version)
	}
	return queued
}
