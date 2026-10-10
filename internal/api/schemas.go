package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/vibemdm/vibemdm/internal/model"
	"github.com/vibemdm/vibemdm/internal/schema"
	"github.com/vibemdm/vibemdm/internal/store"
)

// Schema references are "builtin:<domain>" for bundled schemas or a custom schema UUID.

type schemaView struct {
	ID               string          `json:"id"`
	Name             string          `json:"name"`
	PreferenceDomain string          `json:"preference_domain"`
	Builtin          bool            `json:"builtin"`
	Title            string          `json:"title"`
	Description      string          `json:"description"`
	Properties       int             `json:"properties"`
	Profiles         int             `json:"profiles"`
	Schema           json.RawMessage `json:"schema,omitempty"`
	UpdatedAt        any             `json:"updated_at,omitempty"`
}

// loadSchema resolves a schema reference to its parsed schema, domain and source tag.
func (h *Handler) loadSchema(r *http.Request, ref string) (*schema.Schema, schemaView, error) {
	if strings.HasPrefix(ref, "builtin:") {
		b, ok := schema.BuiltinByDomain(strings.TrimPrefix(ref, "builtin:"))
		if !ok {
			return nil, schemaView{}, store.ErrNotFound
		}
		v := schemaView{ID: ref, Name: b.Schema.Title, PreferenceDomain: b.ID, Builtin: true, Title: b.Schema.Title,
			Description: b.Schema.Description, Properties: len(b.Schema.Properties), Schema: b.Raw}
		return b.Schema, v, nil
	}
	if _, err := uuid.Parse(ref); err != nil {
		return nil, schemaView{}, store.ErrNotFound
	}
	c, err := h.st.GetCustomSchema(r.Context(), ref)
	if err != nil {
		return nil, schemaView{}, err
	}
	s, err := schema.Parse(c.Schema)
	if err != nil {
		return nil, schemaView{}, err
	}
	v := schemaView{ID: c.ID, Name: c.Name, PreferenceDomain: c.PreferenceDomain, Title: s.Title, Description: s.Description,
		Properties: len(s.Properties), Schema: c.Schema, UpdatedAt: c.UpdatedAt}
	return s, v, nil
}

func sourceTag(ref string) string {
	if strings.HasPrefix(ref, "builtin:") {
		return ref
	}
	return "custom:" + ref
}

func (h *Handler) listSchemas(w http.ResponseWriter, r *http.Request) {
	out := []schemaView{}
	for _, b := range schema.Builtins() {
		n, _ := h.st.ProfilesUsingSchema(r.Context(), "builtin:"+b.ID)
		out = append(out, schemaView{ID: "builtin:" + b.ID, Name: b.Schema.Title, PreferenceDomain: b.ID, Builtin: true,
			Title: b.Schema.Title, Description: b.Schema.Description, Properties: len(b.Schema.Properties), Profiles: n})
	}
	customs, err := h.st.ListCustomSchemas(r.Context())
	if err != nil {
		h.fail(w, err, "schemas")
		return
	}
	for _, c := range customs {
		v := schemaView{ID: c.ID, Name: c.Name, PreferenceDomain: c.PreferenceDomain, UpdatedAt: c.UpdatedAt}
		if s, err := schema.Parse(c.Schema); err == nil {
			v.Title, v.Description, v.Properties = s.Title, s.Description, len(s.Properties)
		}
		v.Profiles, _ = h.st.ProfilesUsingSchema(r.Context(), "custom:"+c.ID)
		out = append(out, v)
	}
	writeJSON(w, 200, out)
}

func (h *Handler) getSchema(w http.ResponseWriter, r *http.Request) {
	_, v, err := h.loadSchema(r, r.PathValue("id"))
	if err != nil {
		h.failSchema(w, err)
		return
	}
	v.Profiles, _ = h.st.ProfilesUsingSchema(r.Context(), sourceTag(v.ID))
	writeJSON(w, 200, v)
}

func (h *Handler) failSchema(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "schema not found")
		return
	}
	writeErr(w, 400, err.Error())
}

// saveSchema uploads or replaces a custom schema. The body's "schema" may be a
// JSON object or a string containing JSON (as pasted from a file).
func (h *Handler) saveSchema(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name             string          `json:"name"`
		PreferenceDomain string          `json:"preference_domain"`
		Schema           json.RawMessage `json:"schema"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	raw := []byte(req.Schema)
	var asString string
	if json.Unmarshal(raw, &asString) == nil {
		raw = []byte(asString)
	}
	s, err := schema.Parse(raw)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	domain := strings.TrimSpace(req.PreferenceDomain)
	if domain == "" {
		domain = s.PreferenceDomain
	}
	if err := schema.CheckDomain(domain); err != nil {
		writeErr(w, 400, "preference domain: "+err.Error())
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = s.Title
	}
	if name == "" {
		name = domain
	}
	compact, _ := json.Marshal(json.RawMessage(raw))
	out, err := h.st.SaveCustomSchema(r.Context(), model.CustomSchema{ID: r.PathValue("id"), Name: name, PreferenceDomain: domain,
		Schema: compact, CreatedBy: actor(r)})
	if err != nil {
		h.fail(w, err, "schema")
		return
	}
	h.st.Audit(r.Context(), actor(r), "schema.save", "custom_schema", out.ID, map[string]any{"name": out.Name, "domain": domain})
	code := 200
	if r.PathValue("id") == "" {
		code = 201
	}
	writeJSON(w, code, out)
}

func (h *Handler) deleteSchema(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if strings.HasPrefix(id, "builtin:") {
		writeErr(w, 400, "built-in schemas cannot be deleted")
		return
	}
	if err := h.st.DeleteCustomSchema(r.Context(), id); err != nil {
		h.fail(w, err, "schema")
		return
	}
	h.st.Audit(r.Context(), actor(r), "schema.delete", "custom_schema", id, nil)
	w.WriteHeader(204)
}

type schemaProfileReq struct {
	ProfileID   string         `json:"profile_id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Category    string         `json:"category"`
	Scope       model.Scope    `json:"scope"`
	Values      map[string]any `json:"values"`
}

// buildFromSchema validates values and renders the profile payload.
func (h *Handler) buildFromSchema(r *http.Request, s *schema.Schema, v schemaView, identifier, name, description string, values map[string]any) (map[string]any, string, []schema.FieldError, error) {
	clean, errs := s.Validate(values)
	if len(errs) > 0 {
		return nil, "", errs, nil
	}
	settings, _ := h.st.GetSettings(r.Context())
	payload, err := schema.BuildProfile(schema.ProfileInput{Identifier: identifier, DisplayName: name, Description: description,
		Organization: settings.OrgName, Domain: v.PreferenceDomain, Values: clean})
	return clean, string(payload), nil, err
}

// previewSchemaProfile validates values and returns the payload without saving.
func (h *Handler) previewSchemaProfile(w http.ResponseWriter, r *http.Request) {
	s, v, err := h.loadSchema(r, r.PathValue("id"))
	if err != nil {
		h.failSchema(w, err)
		return
	}
	var req schemaProfileReq
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	name := firstNonEmpty(req.Name, v.Name)
	clean, payload, errs, err := h.buildFromSchema(r, s, v, "com.vibemdm.app.preview", name, req.Description, req.Values)
	if err != nil {
		writeErr(w, 422, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"values": clean, "errors": nonNilErrs(errs), "payload": payload})
}

func nonNilErrs(e []schema.FieldError) []schema.FieldError {
	if e == nil {
		return []schema.FieldError{}
	}
	return e
}

// saveSchemaProfile creates or updates a configuration profile from schema values.
func (h *Handler) saveSchemaProfile(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")
	s, v, err := h.loadSchema(r, ref)
	if err != nil {
		h.failSchema(w, err)
		return
	}
	var req schemaProfileReq
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeErr(w, 400, "profile name required")
		return
	}
	var existing *model.ConfigProfile
	identifier := "com.vibemdm.app." + v.PreferenceDomain + "." + strings.Split(uuid.NewString(), "-")[0]
	if req.ProfileID != "" {
		if existing, err = h.st.GetProfile(r.Context(), req.ProfileID); err != nil {
			h.fail(w, err, "profile")
			return
		}
		identifier = existing.Identifier
	}
	clean, payload, errs, err := h.buildFromSchema(r, s, v, identifier, req.Name, req.Description, req.Values)
	if err != nil {
		writeErr(w, 422, err.Error())
		return
	}
	if len(errs) > 0 {
		writeJSON(w, 400, map[string]any{"error": "some values are invalid", "errors": errs})
		return
	}
	p := model.ConfigProfile{Name: req.Name, Description: req.Description, Category: firstNonEmpty(req.Category, "Applications"),
		Identifier: identifier, Payload: payload, Scope: req.Scope}
	if err := normalizeProfilePayload(&p); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	var out *model.ConfigProfile
	reinstall := false
	if existing == nil {
		out, err = h.st.CreateProfile(r.Context(), p)
	} else {
		p.ID = existing.ID
		reinstall = existing.Payload != p.Payload
		out, err = h.st.UpdateProfile(r.Context(), p)
	}
	if err != nil {
		h.fail(w, err, "profile")
		return
	}
	if err := h.st.SetProfileSchema(r.Context(), out.ID, sourceTag(ref), clean); err != nil {
		h.fail(w, err, "profile")
		return
	}
	out.SchemaSource, out.SchemaValues = sourceTag(ref), clean
	h.st.Audit(r.Context(), actor(r), "profile.schema_save", "profile", out.ID, map[string]any{"name": out.Name, "schema": sourceTag(ref)})
	h.syncProfileAsync(out, actor(r), reinstall)
	code := 200
	if existing == nil {
		code = 201
	}
	writeJSON(w, code, out)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
