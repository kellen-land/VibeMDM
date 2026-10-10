package api

import (
	"compress/gzip"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/vibemdm/vibemdm/internal/auth"
	"github.com/vibemdm/vibemdm/internal/model"
	"github.com/vibemdm/vibemdm/internal/store"
)

func logFilterFrom(r *http.Request) store.LogFilter {
	q := r.URL.Query()
	f := store.LogFilter{Source: q.Get("source"), MinLevel: q.Get("level"), Component: q.Get("component"), ComputerID: q.Get("computer_id"),
		Q: q.Get("q"), Limit: qInt(r, "limit", 200)}
	f.SinceID, _ = strconv.ParseInt(q.Get("since_id"), 10, 64)
	f.BeforeID, _ = strconv.ParseInt(q.Get("before_id"), 10, 64)
	for key, dst := range map[string]**time.Time{"from": &f.From, "to": &f.To} {
		if v := q.Get(key); v != "" {
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				*dst = &t
			}
		}
	}
	return f
}

// listLogs searches and tails server or agent logs.
func (h *Handler) listLogs(w http.ResponseWriter, r *http.Request) {
	f := logFilterFrom(r)
	items, err := h.st.QueryLogs(r.Context(), f)
	if err != nil {
		h.fail(w, err, "logs")
		return
	}
	out := map[string]any{"items": items}
	if len(items) > 0 {
		if f.SinceID > 0 {
			out["last_id"] = items[len(items)-1].ID
		} else {
			out["last_id"] = items[0].ID
			out["next_before_id"] = items[len(items)-1].ID
		}
	} else {
		out["last_id"] = f.SinceID
	}
	writeJSON(w, 200, out)
}

func (h *Handler) logComponents(w http.ResponseWriter, r *http.Request) {
	comps, err := h.st.LogComponents(r.Context(), r.URL.Query().Get("source"))
	if err != nil {
		h.fail(w, err, "logs")
		return
	}
	writeJSON(w, 200, comps)
}

// exportLogs streams matching logs as gzipped NDJSON. Admin-only and audited.
func (h *Handler) exportLogs(w http.ResponseWriter, r *http.Request) {
	f := logFilterFrom(r)
	if f.Source != "agent" {
		f.Source = "server"
	}
	h.st.Audit(r.Context(), actor(r), "logs.export", "logs", f.Source, map[string]any{"q": f.Q, "level": f.MinLevel, "computer_id": f.ComputerID})
	name := fmt.Sprintf("vibemdm-%s-logs-%s.ndjson.gz", f.Source, time.Now().UTC().Format("20060102-150405"))
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, name))
	gz := gzip.NewWriter(w)
	if _, err := h.st.ExportLogs(r.Context(), f, gz); err != nil {
		h.log.Warn("log export interrupted", "err", err)
	}
	gz.Close()
}

// listLogArchives lists daily archive files.
func (h *Handler) listLogArchives(w http.ResponseWriter, r *http.Request) {
	dir := filepath.Join(h.cfg.DataDir, "log-archive")
	entries, _ := os.ReadDir(dir)
	type archive struct {
		Name string    `json:"name"`
		Size int64     `json:"size_bytes"`
		At   time.Time `json:"modified_at"`
	}
	out := []archive{}
	for _, e := range entries {
		if fi, err := e.Info(); err == nil && !e.IsDir() && filepath.Ext(e.Name()) == ".gz" {
			out = append(out, archive{e.Name(), fi.Size(), fi.ModTime()})
		}
	}
	writeJSON(w, 200, out)
}

func (h *Handler) downloadLogArchive(w http.ResponseWriter, r *http.Request) {
	name := filepath.Base(r.PathValue("name"))
	path := filepath.Join(h.cfg.DataDir, "log-archive", name)
	if filepath.Ext(name) != ".gz" {
		writeErr(w, 400, "invalid archive name")
		return
	}
	if _, err := os.Stat(path); err != nil {
		writeErr(w, 404, "archive not found")
		return
	}
	h.st.Audit(r.Context(), actor(r), "logs.archive_download", "logs", name, nil)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, name))
	http.ServeFile(w, r, path)
}

// ---- diagnostics ----

func (h *Handler) requestDiagnostics(w http.ResponseWriter, r *http.Request) {
	comp, err := h.st.GetComputer(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "computer")
		return
	}
	if !comp.AgentEnrolled {
		writeErr(w, 409, "the VibeMDM agent is not installed on this computer")
		return
	}
	b, err := h.st.CreateDiagnosticBundle(r.Context(), comp.ID, actor(r))
	if err != nil {
		h.fail(w, err, "diagnostics")
		return
	}
	if _, err := h.st.CreateTask(r.Context(), comp.ID, "collect_diagnostics", map[string]any{"bundle_id": b.ID}, actor(r)); err != nil {
		h.fail(w, err, "task")
		return
	}
	h.notifier.Notify(comp.ID)
	h.st.Audit(r.Context(), actor(r), "diagnostics.request", "computer", comp.ID, map[string]any{"bundle_id": b.ID})
	writeJSON(w, 201, b)
}

func (h *Handler) listDiagnostics(w http.ResponseWriter, r *http.Request) {
	items, err := h.st.ListDiagnosticBundles(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "diagnostics")
		return
	}
	writeJSON(w, 200, listResp[model.DiagnosticBundle]{Items: items, Total: len(items)})
}

func (h *Handler) downloadDiagnostics(w http.ResponseWriter, r *http.Request) {
	b, err := h.st.GetDiagnosticBundle(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "diagnostics")
		return
	}
	if b.Status != "uploaded" {
		writeErr(w, 409, "this bundle has not been uploaded")
		return
	}
	h.st.Audit(r.Context(), auth.FromContext(r.Context()).Username, "diagnostics.download", "computer", b.ComputerID, map[string]any{"bundle_id": b.ID})
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, b.Filename))
	http.ServeFile(w, r, filepath.Join(h.cfg.DataDir, "diagnostics", b.ID+".tar.gz"))
}
