package api

import (
	"errors"
	"io"
	"net/http"

	"github.com/vibemdm/vibemdm/internal/agentinstall"
	"github.com/vibemdm/vibemdm/internal/auth"
	"github.com/vibemdm/vibemdm/internal/store"
)

// agentPackage reports the uploaded package, the signing CA and the auto-install setting.
func (h *Handler) agentPackage(w http.ResponseWriter, r *http.Request) {
	settings, _ := h.st.GetSettings(r.Context())
	out := map[string]any{"auto_install": settings.AutoInstallAgent, "package": nil}
	if p, err := h.agentInstall.Package(r.Context()); err == nil {
		out["package"] = p
	} else if !errors.Is(err, store.ErrNotFound) {
		h.fail(w, err, "agent package")
		return
	}
	if ca, _, err := h.agentInstall.CA(r.Context()); err == nil {
		out["ca_fingerprint"] = agentinstall.Fingerprint(ca)
	}
	writeJSON(w, 200, out)
}

// uploadAgentPackage stores a signed agent package (raw body). ?version= is required.
func (h *Handler) uploadAgentPackage(w http.ResponseWriter, r *http.Request) {
	p, err := h.agentInstall.Upload(r.Context(), http.MaxBytesReader(w, r.Body, 201<<20), r.URL.Query().Get("version"), actor(r))
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	h.st.Audit(r.Context(), actor(r), "agent.package_uploaded", "settings", "", map[string]any{"version": p.Version, "sha256": p.SHA256, "signed_by": p.SignedBy})
	h.agentPackage(w, r)
}

// agentPackageSettings turns automatic installation after MDM enrollment on or off.
func (h *Handler) agentPackageSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AutoInstall bool `json:"auto_install"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	settings, err := h.st.GetSettings(r.Context())
	if err != nil {
		h.fail(w, err, "settings")
		return
	}
	settings.AutoInstallAgent = req.AutoInstall
	if err := h.st.SaveSettings(r.Context(), settings); err != nil {
		h.fail(w, err, "settings")
		return
	}
	h.st.Audit(r.Context(), actor(r), "agent.auto_install", "settings", "", map[string]any{"enabled": req.AutoInstall})
	h.agentPackage(w, r)
}

// signAgentPackageCSR issues a code-signing certificate from the package-signing CA for a
// CSR made on the build Mac. The private key never reaches the server.
func (h *Handler) signAgentPackageCSR(w http.ResponseWriter, r *http.Request) {
	csr, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<10))
	if err != nil {
		writeErr(w, 400, "request too large")
		return
	}
	caCert, caKey, err := h.agentInstall.CA(r.Context())
	if err != nil {
		h.fail(w, err, "package-signing CA")
		return
	}
	chain, err := agentinstall.SignCSR(csr, caCert, caKey)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	h.st.Audit(r.Context(), actor(r), "agent.signing_cert_issued", "settings", "", map[string]any{"ip": auth.ClientIP(r)})
	w.Header().Set("Content-Type", "application/x-pem-file")
	io.WriteString(w, chain)
}

// computerAgentInstall returns a Mac's latest MDM agent install attempt.
func (h *Handler) computerAgentInstall(w http.ResponseWriter, r *http.Request) {
	a, err := h.st.LatestAgentInstall(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "agent install")
		return
	}
	writeJSON(w, 200, map[string]any{"attempt": a})
}

// installAgentMDM sends the agent package to one Mac now.
func (h *Handler) installAgentMDM(w http.ResponseWriter, r *http.Request) {
	comp, err := h.st.GetComputer(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "computer")
		return
	}
	if err := h.agentInstall.Install(r.Context(), comp, actor(r)); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	h.computerAgentInstall(w, r)
}
