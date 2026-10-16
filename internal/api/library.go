package api

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/vibemdm/vibemdm/internal/assets"
	"github.com/vibemdm/vibemdm/internal/model"
	"github.com/vibemdm/vibemdm/internal/store"
)

// ---- scripts ----

func (h *Handler) listScripts(w http.ResponseWriter, r *http.Request) {
	items, err := h.st.ListScripts(r.Context(), r.URL.Query().Get("contents") == "1")
	if err != nil {
		h.fail(w, err, "scripts")
		return
	}
	for i := range items {
		redactScriptFor(r, &items[i])
	}
	writeJSON(w, 200, listResp[model.Script]{Items: items, Total: len(items)})
}

func (h *Handler) getScript(w http.ResponseWriter, r *http.Request) {
	s, err := h.st.GetScript(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "script")
		return
	}
	redactScriptFor(r, s)
	writeJSON(w, 200, s)
}

func (h *Handler) createScript(w http.ResponseWriter, r *http.Request) {
	var s model.Script
	if err := decode(r, &s); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if strings.TrimSpace(s.Name) == "" {
		writeErr(w, 400, "name required")
		return
	}
	out, err := h.st.CreateScript(r.Context(), s)
	if err != nil {
		h.fail(w, err, "script")
		return
	}
	h.st.Audit(r.Context(), actor(r), "script.create", "script", out.ID, map[string]any{"name": out.Name})
	writeJSON(w, 201, out)
}

func (h *Handler) updateScript(w http.ResponseWriter, r *http.Request) {
	var s model.Script
	if err := decode(r, &s); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	s.ID = r.PathValue("id")
	out, err := h.st.UpdateScript(r.Context(), s)
	if err != nil {
		h.fail(w, err, "script")
		return
	}
	h.st.Audit(r.Context(), actor(r), "script.update", "script", out.ID, map[string]any{"name": out.Name})
	writeJSON(w, 200, out)
}

func (h *Handler) deleteScript(w http.ResponseWriter, r *http.Request) {
	if err := h.st.DeleteScript(r.Context(), r.PathValue("id")); err != nil {
		h.fail(w, err, "script")
		return
	}
	h.st.Audit(r.Context(), actor(r), "script.delete", "script", r.PathValue("id"), nil)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// runScript queues a script on computers and/or groups.
func (h *Handler) runScript(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ComputerIDs []string `json:"computer_ids"`
		GroupIDs    []string `json:"group_ids"`
		Parameters  []string `json:"parameters"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	targets, err := h.st.ResolveScope(r.Context(), model.Scope{Targets: model.ScopeSelection{ComputerIDs: req.ComputerIDs, GroupIDs: req.GroupIDs}})
	if err != nil {
		h.fail(w, err, "scope")
		return
	}
	queued, skipped := 0, []string{}
	for _, id := range targets {
		c, err := h.st.GetComputer(r.Context(), id)
		if err != nil {
			continue
		}
		if _, err := h.queueTask(r, c, "run_script", r.PathValue("id"), req.Parameters, "", "", "", ""); err != nil {
			skipped = append(skipped, c.Name+": "+err.Error())
			continue
		}
		queued++
	}
	writeJSON(w, 200, map[string]any{"queued": queued, "skipped": skipped})
}

// ---- packages ----

var safeFilename = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func (h *Handler) listPackages(w http.ResponseWriter, r *http.Request) {
	items, err := h.st.ListPackages(r.Context())
	if err != nil {
		h.fail(w, err, "packages")
		return
	}
	writeJSON(w, 200, listResp[model.Package]{Items: items, Total: len(items)})
}

func (h *Handler) getPackage(w http.ResponseWriter, r *http.Request) {
	p, err := h.st.GetPackage(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "package")
		return
	}
	writeJSON(w, 200, p)
}

func (h *Handler) uploadPackage(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 8<<30)
	mr, err := r.MultipartReader()
	if err != nil {
		writeErr(w, 400, "multipart form required")
		return
	}
	var pkg model.Package
	var fileSaved bool
	dir := filepath.Join(h.cfg.AssetsDir, assets.Packages)
	os.MkdirAll(dir, 0o755)
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		switch part.FormName() {
		case "name":
			b, _ := io.ReadAll(io.LimitReader(part, 4096))
			pkg.Name = strings.TrimSpace(string(b))
		case "category":
			b, _ := io.ReadAll(io.LimitReader(part, 4096))
			pkg.Category = strings.TrimSpace(string(b))
		case "info":
			b, _ := io.ReadAll(io.LimitReader(part, 65536))
			pkg.Info = string(b)
		case "notes":
			b, _ := io.ReadAll(io.LimitReader(part, 65536))
			pkg.Notes = string(b)
		case "priority":
			b, _ := io.ReadAll(io.LimitReader(part, 16))
			fmt.Sscanf(strings.TrimSpace(string(b)), "%d", &pkg.Priority)
		case "file":
			fn := safeFilename.ReplaceAllString(filepath.Base(part.FileName()), "_")
			if fn == "" || fn == "." {
				writeErr(w, 400, "invalid filename")
				return
			}
			ext := strings.ToLower(filepath.Ext(fn))
			if ext != ".pkg" && ext != ".dmg" && ext != ".mpkg" && ext != ".zip" {
				writeErr(w, 400, "only .pkg, .mpkg, .dmg and .zip files are supported")
				return
			}
			dst := filepath.Join(dir, fn)
			f, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
			if errors.Is(err, fs.ErrExist) {
				writeErr(w, 409, "a package file with that name already exists")
				return
			}
			if err != nil {
				h.fail(w, err, "package file")
				return
			}
			hsh := sha256.New()
			n, err := io.Copy(io.MultiWriter(f, hsh), part)
			f.Close()
			if err != nil {
				os.Remove(dst)
				h.fail(w, err, "package upload")
				return
			}
			pkg.Filename, pkg.SizeBytes, pkg.SHA256 = fn, n, hex.EncodeToString(hsh.Sum(nil))
			fileSaved = true
		}
	}
	if !fileSaved {
		writeErr(w, 400, "file required")
		return
	}
	if pkg.Name == "" {
		pkg.Name = pkg.Filename
	}
	out, err := h.st.CreatePackage(r.Context(), pkg)
	if err != nil {
		os.Remove(filepath.Join(dir, pkg.Filename))
		h.fail(w, err, "package")
		return
	}
	h.st.Audit(r.Context(), actor(r), "package.upload", "package", out.ID, map[string]any{"name": out.Name, "size": out.SizeBytes})
	writeJSON(w, 201, out)
}

func (h *Handler) updatePackage(w http.ResponseWriter, r *http.Request) {
	var p model.Package
	if err := decode(r, &p); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	p.ID = r.PathValue("id")
	out, err := h.st.UpdatePackage(r.Context(), p)
	if err != nil {
		h.fail(w, err, "package")
		return
	}
	h.st.Audit(r.Context(), actor(r), "package.update", "package", out.ID, map[string]any{"name": out.Name})
	writeJSON(w, 200, out)
}

func (h *Handler) deletePackage(w http.ResponseWriter, r *http.Request) {
	p, err := h.st.GetPackage(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "package")
		return
	}
	if err := h.st.DeletePackage(r.Context(), p.ID); err != nil {
		h.fail(w, err, "package")
		return
	}
	os.Remove(filepath.Join(h.cfg.AssetsDir, assets.Packages, filepath.Base(p.Filename)))
	h.st.Audit(r.Context(), actor(r), "package.delete", "package", p.ID, map[string]any{"name": p.Name})
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (h *Handler) downloadPackage(w http.ResponseWriter, r *http.Request) {
	p, err := h.st.GetPackage(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "package")
		return
	}
	attachmentHeaders(w, filepath.Base(p.Filename), "application/octet-stream")
	http.ServeFile(w, r, filepath.Join(h.cfg.AssetsDir, assets.Packages, filepath.Base(p.Filename)))
}

// ---- extension attributes ----

func (h *Handler) listEAs(w http.ResponseWriter, r *http.Request) {
	items, err := h.st.ListExtensionAttributes(r.Context())
	if err != nil {
		h.fail(w, err, "extension attributes")
		return
	}
	for i := range items {
		redactEAFor(r, &items[i])
	}
	writeJSON(w, 200, listResp[model.ExtensionAttribute]{Items: items, Total: len(items)})
}

func (h *Handler) getEA(w http.ResponseWriter, r *http.Request) {
	e, err := h.st.GetExtensionAttribute(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "extension attribute")
		return
	}
	redactEAFor(r, e)
	writeJSON(w, 200, e)
}

func (h *Handler) createEA(w http.ResponseWriter, r *http.Request) {
	var e model.ExtensionAttribute
	if err := decode(r, &e); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if strings.TrimSpace(e.Name) == "" {
		writeErr(w, 400, "name required")
		return
	}
	out, err := h.st.CreateExtensionAttribute(r.Context(), e)
	if err != nil {
		h.fail(w, err, "extension attribute")
		return
	}
	h.st.Audit(r.Context(), actor(r), "ea.create", "extension_attribute", out.ID, map[string]any{"name": out.Name})
	writeJSON(w, 201, out)
}

func (h *Handler) updateEA(w http.ResponseWriter, r *http.Request) {
	var e model.ExtensionAttribute
	if err := decode(r, &e); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	e.ID = r.PathValue("id")
	out, err := h.st.UpdateExtensionAttribute(r.Context(), e)
	if err != nil {
		h.fail(w, err, "extension attribute")
		return
	}
	h.st.Audit(r.Context(), actor(r), "ea.update", "extension_attribute", out.ID, map[string]any{"name": out.Name})
	writeJSON(w, 200, out)
}

func (h *Handler) deleteEA(w http.ResponseWriter, r *http.Request) {
	if err := h.st.DeleteExtensionAttribute(r.Context(), r.PathValue("id")); err != nil {
		h.fail(w, err, "extension attribute")
		return
	}
	h.st.Audit(r.Context(), actor(r), "ea.delete", "extension_attribute", r.PathValue("id"), nil)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

var _ = store.ErrNotFound
