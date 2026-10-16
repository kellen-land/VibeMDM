package mdmsvc

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/micromdm/nanomdm/mdm"
	"github.com/micromdm/nanomdm/service"
	"github.com/micromdm/nanomdm/service/certauth"
)

// errEnrollmentHeld is returned when another identity certificate tries to Authenticate as a
// device that still holds a live enrollment.
var errEnrollmentHeld = errors.New("enrollment is held by another identity certificate")

// ReenrollChecker decides whether an Authenticate may bind a new certificate to an enrollment.
type ReenrollChecker interface {
	MDMReenrollAllowed(ctx context.Context, id, certHash string) (bool, error)
	Audit(ctx context.Context, actor, action, targetType, targetID string, details map[string]any)
}

// ReenrollGuard sits in front of NanoMDM's certauth. certauth treats every Authenticate as a
// new enrollment and binds whatever certificate it carries to the UDID in the message, so on
// its own any SCEP-issued certificate could re-enroll as an existing device. The guard refuses
// that while the existing enrollment is live (see store.MDMReenrollAllowed).
type ReenrollGuard struct {
	service.CheckinAndCommandService
	st  ReenrollChecker
	log *slog.Logger
}

// NewReenrollGuard wraps next (normally the certauth service).
func NewReenrollGuard(next service.CheckinAndCommandService, st ReenrollChecker, logger *slog.Logger) *ReenrollGuard {
	return &ReenrollGuard{CheckinAndCommandService: next, st: st, log: logger}
}

// Authenticate checks the certificate before certauth associates it.
func (g *ReenrollGuard) Authenticate(r *mdm.Request, m *mdm.Authenticate) error {
	if r.Certificate == nil {
		return g.CheckinAndCommandService.Authenticate(r, m)
	}
	id := m.UDID
	if id == "" {
		id = m.EnrollmentID
	}
	ok, err := g.st.MDMReenrollAllowed(r.Context(), id, certauth.HashCert(r.Certificate))
	if err != nil {
		return err
	}
	if !ok {
		g.log.Warn("mdm: refused Authenticate for an enrolled device from a different certificate", "udid", id, "serial", m.SerialNumber)
		g.st.Audit(r.Context(), "system", "mdm.enroll_refused", "device", id, map[string]any{"udid": id, "serial": m.SerialNumber})
		return service.NewHTTPStatusError(http.StatusForbidden, errEnrollmentHeld)
	}
	return g.CheckinAndCommandService.Authenticate(r, m)
}
