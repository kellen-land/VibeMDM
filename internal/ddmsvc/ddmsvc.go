// Package ddmsvc serves Declarative Device Management check-ins for NanoMDM.
package ddmsvc

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/micromdm/nanomdm/mdm"
	"github.com/micromdm/nanomdm/service"

	"github.com/vibemdm/vibemdm/internal/ddm"
	"github.com/vibemdm/vibemdm/internal/store"
)

// Handler implements service.DeclarativeManagement.
type Handler struct {
	st  *store.Store
	log *slog.Logger
}

// New creates a handler.
func New(st *store.Store, logger *slog.Logger) *Handler { return &Handler{st: st, log: logger} }

func notFound(err error) error { return service.NewHTTPStatusError(http.StatusNotFound, err) }

// DeclarativeManagement answers the device's DDM requests.
func (h *Handler) DeclarativeManagement(r *mdm.Request, m *mdm.DeclarativeManagement) ([]byte, error) {
	ctx := r.Context()
	if r.EnrollID == nil {
		return nil, errors.New("missing enrollment id")
	}
	// User-channel DDM isn't used by VibeMDM: answer with an empty set.
	var decls []ddm.Declaration
	compID := ""
	if r.ParentID == "" {
		comp, err := h.st.GetComputerByUDID(ctx, r.ID)
		if err != nil {
			return nil, notFound(fmt.Errorf("unknown device %s", r.ID))
		}
		compID = comp.ID
		if decls, err = h.st.DeclarationsFor(ctx, comp); err != nil {
			return nil, err
		}
	}
	token := ddm.DeclarationsToken(decls)
	switch ep := m.Endpoint; {
	case ep == "tokens":
		if compID != "" {
			h.st.MarkDDMServed(ctx, compID, token)
		}
		return ddm.TokensResponse(token, time.Now())
	case ep == "declaration-items":
		if compID != "" {
			h.st.MarkDDMServed(ctx, compID, token)
		}
		return ddm.ItemsResponse(decls)
	case ep == "status":
		if compID != "" && len(m.Data) > 0 {
			if err := h.st.ApplyDDMStatus(ctx, compID, m.Data); err != nil {
				h.log.Warn("storing DDM status report", "err", err, "udid", r.ID)
				return nil, service.NewHTTPStatusError(http.StatusBadRequest, err)
			}
		}
		return nil, nil
	case strings.HasPrefix(ep, "declaration/"):
		d, err := ddm.Find(decls, ep)
		if err != nil {
			return nil, notFound(err)
		}
		return jsonMarshal(d)
	default:
		return nil, notFound(fmt.Errorf("unsupported DDM endpoint %q", ep))
	}
}
