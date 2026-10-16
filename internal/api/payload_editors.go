package api

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"

	"github.com/vibemdm/vibemdm/internal/model"
	"github.com/vibemdm/vibemdm/internal/schema"
)

// Payload editors (Jamf Pro's payload list, generated from Apple's device-management
// repository) for Computer and Mobile Device Configuration Profiles.

type payloadEditorView struct {
	schema.PayloadDef
	Schema json.RawMessage `json:"schema"`
}

var (
	editorsMu   sync.Mutex
	editorsGzip = map[string][]byte{}
)

// listPayloadEditors serves GET /api/v1/profile-payloads?platform=macos|ios.
func (h *Handler) listPayloadEditors(w http.ResponseWriter, r *http.Request) {
	plat := r.URL.Query().Get("platform")
	if plat != schema.PlatformIOS && plat != schema.PlatformMacOS {
		writeErr(w, 400, "platform must be macos or ios")
		return
	}
	writePayloadEditors(w, r, plat)
}

// writePayloadEditors sends a platform's editors (several hundred KB of JSON), gzipped
// once and kept, since the console loads them whenever a profile is opened.
func writePayloadEditors(w http.ResponseWriter, r *http.Request, plat string) {
	editorsMu.Lock()
	body, ok := editorsGzip[plat]
	if !ok {
		ps := schema.Payloads(plat)
		out := make([]payloadEditorView, 0, len(ps))
		for _, p := range ps {
			out = append(out, payloadEditorView{PayloadDef: p, Schema: p.Raw})
		}
		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		json.NewEncoder(zw).Encode(listResp[payloadEditorView]{Items: out, Total: len(out)})
		zw.Close()
		body = buf.Bytes()
		editorsGzip[plat] = body
	}
	editorsMu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, max-age=300")
	w.Header().Set("Vary", "Accept-Encoding")
	if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		w.Header().Set("Content-Encoding", "gzip")
		w.Write(body)
		return
	}
	zr, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		writeErr(w, 500, "payload editors")
		return
	}
	io.Copy(w, zr)
}

// buildPayloadXML renders editor payloads; perrs is set when values are invalid.
func (h *Handler) buildPayloadXML(ctx context.Context, plat, identifier, name, description, level string, payloads []model.PayloadValues) (string, []schema.PayloadErrors, error) {
	settings, _ := h.st.GetSettings(ctx)
	in := schema.PayloadProfileInput{Platform: plat, Identifier: identifier, DisplayName: name, Description: description,
		Organization: settings.OrgName, Level: level}
	for _, p := range payloads {
		in.Payloads = append(in.Payloads, schema.PayloadValues{Type: p.Type, UUID: p.UUID, Values: p.Values})
	}
	out, perrs, err := schema.BuildPayloadProfile(in)
	return string(out), perrs, err
}

// previewProfile serves POST /api/v1/profiles/preview (the XML a payload-editor profile renders to).
func (h *Handler) previewProfile(w http.ResponseWriter, r *http.Request) {
	var req model.ConfigProfile
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	xml, perrs, err := h.buildPayloadXML(r.Context(), schema.PlatformMacOS, "com.vibemdm.profile.preview", firstNonEmpty(req.Name, "New profile"), req.Description, req.Level, req.Payloads)
	if err != nil {
		writeJSON(w, 200, map[string]any{"payload": "", "error": err.Error(), "payload_errors": []schema.PayloadErrors{}})
		return
	}
	if perrs == nil {
		perrs = []schema.PayloadErrors{}
	}
	writeJSON(w, 200, map[string]any{"payload": xml, "payload_errors": perrs})
}

// legacySecret catches secrets in editors without x-secret marks (older saved values).
var legacySecret = regexp.MustCompile(`(?i)(password|secret|challenge|presharedkey|passphrase|passcode|privatekey|license)`)

// redactPayloads hides fields the editor marks secret (passwords, shared secrets, SCEP
// challenges, identity files) from viewers.
func redactPayloads(plat string, ps []model.PayloadValues) {
	for i := range ps {
		def, ok := schema.PayloadByType(plat, ps[i].Type)
		identity := ps[i].Type == "com.apple.security.pkcs12"
		ps[i].Values = redactTree(ps[i].Values, nil, func(path []string, key string, v any) bool {
			if identity && len(path) == 1 && key == "PayloadContent" {
				return true
			}
			if _, isStr := v.(string); isStr && legacySecret.MatchString(key) {
				return true
			}
			return ok && def.IsSecret(path)
		}).(map[string]any)
	}
}

func redactTree(v any, path []string, secret func(path []string, key string, v any) bool) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, x := range t {
			p := append(append([]string{}, path...), k)
			if secret(p, k, x) {
				out[k] = "[redacted]"
				continue
			}
			out[k] = redactTree(x, p, secret)
		}
		return out
	case []any:
		out := make([]any, len(t))
		p := append(append([]string{}, path...), "[]")
		for i, x := range t {
			out[i] = redactTree(x, p, secret)
		}
		return out
	}
	return v
}
