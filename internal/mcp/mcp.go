// Package mcp exposes VibeMDM to AI agents (Claude Code, Claude Desktop, the
// Claude API's MCP connector, and other MCP clients) over the Model Context
// Protocol's Streamable HTTP transport at /mcp.
//
// Security model:
//   - off unless an admin enables it (Settings → MCP server); returns 404 otherwise
//   - authenticated only with VibeMDM API tokens (Authorization: Bearer vmdm_…);
//     browser cookies are never accepted, so web pages cannot drive it (no CSRF)
//   - requests carrying an Origin header must come from this server's own origin
//     (DNS-rebinding protection, as the MCP spec requires)
//   - read tools return summaries and never secrets (no FileVault keys, SCEP
//     challenge, enrollment secret, profile payloads, script bodies or raw MDM plists)
//   - write tools are a short list of non-destructive actions, offered only to
//     admin tokens and only after an admin also enables "allow write tools";
//     erase, lock, remote shell and arbitrary commands are never exposed
//   - every tool call is audited and calls are rate limited per token
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/vibemdm/vibemdm/internal/auth"
	"github.com/vibemdm/vibemdm/internal/store"
)

// Protocol versions this server understands (newest first).
var supportedVersions = []string{"2025-11-25", "2025-06-18", "2025-03-26"}

// Server handles MCP requests.
type Server struct {
	st      *store.Store
	auth    *auth.Authenticator
	tools   []*Tool
	byName  map[string]*Tool
	version string
	log     *slog.Logger
	limiter *rateLimiter
}

// Tool is one MCP tool.
type Tool struct {
	Name        string
	Title       string
	Description string
	Input       map[string]any // JSON Schema for arguments
	Write       bool           // changes state: admin token + MCPAllowWrites required
	Destructive bool
	Idempotent  bool
	Run         func(ctx context.Context, p *auth.Principal, args map[string]any) (any, error)
}

// New creates the server with the given tools.
func New(st *store.Store, a *auth.Authenticator, version string, logger *slog.Logger, tools []*Tool) *Server {
	s := &Server{st: st, auth: a, version: version, log: logger, tools: tools, byName: map[string]*Tool{}, limiter: newRateLimiter(120, time.Minute)}
	for _, t := range tools {
		s.byName[t.Name] = t
	}
	return s
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func httpError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func sameHost(origin, host string) bool {
	o := strings.TrimPrefix(strings.TrimPrefix(origin, "https://"), "http://")
	if i := strings.IndexByte(o, '/'); i >= 0 {
		o = o[:i]
	}
	return strings.EqualFold(o, host)
}

// ServeHTTP implements the Streamable HTTP transport (JSON responses, no SSE stream).
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	settings, err := s.st.GetSettings(r.Context())
	if err != nil || !settings.MCPEnabled {
		http.NotFound(w, r)
		return
	}
	if o := r.Header.Get("Origin"); o != "" && !sameHost(o, r.Host) {
		httpError(w, http.StatusForbidden, "cross-origin MCP requests are not allowed")
		return
	}
	if r.Method != http.MethodPost {
		// No server-initiated stream: GET (SSE) and DELETE (sessions) are not offered.
		w.Header().Set("Allow", "POST")
		httpError(w, http.StatusMethodNotAllowed, "use POST")
		return
	}
	p := s.auth.Resolve(r)
	if p == nil || p.Via != "token" {
		w.Header().Set("WWW-Authenticate", `Bearer realm="vibemdm", error="invalid_token"`)
		httpError(w, http.StatusUnauthorized, "an API token is required (Authorization: Bearer vmdm_…)")
		return
	}
	if !s.limiter.allow(p.UserID) {
		httpError(w, http.StatusTooManyRequests, "rate limit exceeded; slow down")
		return
	}
	body := http.MaxBytesReader(w, r.Body, 1<<20)
	var raw json.RawMessage
	if err := json.NewDecoder(body).Decode(&raw); err != nil {
		writeJSON(w, 400, rpcResponse{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{-32700, "parse error"}})
		return
	}
	if len(raw) > 0 && raw[0] == '[' {
		writeJSON(w, 400, rpcResponse{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{-32600, "batching is not supported"}})
		return
	}
	var req rpcRequest
	if err := json.Unmarshal(raw, &req); err != nil || req.JSONRPC != "2.0" || req.Method == "" {
		writeJSON(w, 400, rpcResponse{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{-32600, "invalid request"}})
		return
	}
	if len(req.ID) == 0 { // notification or response: acknowledge
		w.WriteHeader(http.StatusAccepted)
		return
	}
	result, rerr := s.dispatch(r.Context(), p, settings.MCPAllowWrites, req)
	resp := rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: result, Error: rerr}
	if v := r.Header.Get("MCP-Protocol-Version"); v != "" {
		w.Header().Set("MCP-Protocol-Version", v)
	}
	writeJSON(w, 200, resp)
}

func (s *Server) allowed(t *Tool, p *auth.Principal, writes bool) bool {
	return !t.Write || (writes && p.IsAdmin())
}

func (s *Server) dispatch(ctx context.Context, p *auth.Principal, writes bool, req rpcRequest) (any, *rpcError) {
	switch req.Method {
	case "initialize":
		var params struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		json.Unmarshal(req.Params, &params)
		version := supportedVersions[0]
		for _, v := range supportedVersions {
			if v == params.ProtocolVersion {
				version = v
			}
		}
		mode := "read-only"
		if writes && p.IsAdmin() {
			mode = "read and limited write"
		}
		return map[string]any{
			"protocolVersion": version,
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":      map[string]any{"name": "vibemdm", "title": "VibeMDM", "version": s.version},
			"instructions": "VibeMDM is a self-hosted Apple MDM. Use these tools to inspect the Mac fleet (inventory, health issues, " +
				"compliance, software updates, policies, logs, MDM commands) and, when permitted, to take safe actions. Computers can be " +
				"referenced by ID, serial number or exact name. Access for this token: " + mode + ".",
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		list := []map[string]any{}
		for _, t := range s.tools {
			if !s.allowed(t, p, writes) {
				continue
			}
			list = append(list, map[string]any{
				"name": t.Name, "title": t.Title, "description": t.Description, "inputSchema": t.Input,
				"annotations": map[string]any{"title": t.Title, "readOnlyHint": !t.Write, "destructiveHint": t.Destructive,
					"idempotentHint": t.Idempotent, "openWorldHint": false},
			})
		}
		return map[string]any{"tools": list}, nil
	case "tools/call":
		var params struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return nil, &rpcError{-32602, "invalid params"}
		}
		t, ok := s.byName[params.Name]
		if !ok || !s.allowed(t, p, writes) {
			return nil, &rpcError{-32602, "unknown tool: " + params.Name}
		}
		if params.Arguments == nil {
			params.Arguments = map[string]any{}
		}
		s.st.Audit(ctx, p.Username, "mcp.tool", "mcp_tool", t.Name, map[string]any{"args": auditArgs(params.Arguments), "write": t.Write})
		cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		out, err := t.Run(cctx, p, params.Arguments)
		if err != nil {
			s.log.Info("mcp tool error", "tool", t.Name, "token", p.Username, "err", err)
			return toolResult(map[string]any{"error": err.Error()}, true), nil
		}
		return toolResult(out, false), nil
	}
	return nil, &rpcError{-32601, "method not found: " + req.Method}
}

func toolResult(v any, isErr bool) map[string]any {
	b, _ := json.MarshalIndent(v, "", "  ")
	res := map[string]any{"content": []any{map[string]any{"type": "text", "text": string(b)}}, "isError": isErr}
	if m, ok := v.(map[string]any); ok {
		res["structuredContent"] = m
	}
	return res
}

// auditArgs records argument names and short scalar values only.
func auditArgs(args map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range args {
		switch t := v.(type) {
		case string:
			if len(t) > 80 {
				t = t[:80] + "…"
			}
			out[k] = t
		case float64, bool:
			out[k] = t
		default:
			out[k] = "…"
		}
	}
	return out
}

// ---- argument helpers ----

var errArg = errors.New("invalid arguments")

func argString(args map[string]any, key string, required bool) (string, error) {
	v, ok := args[key]
	if !ok || v == nil {
		if required {
			return "", fmt.Errorf("%s is required", key)
		}
		return "", nil
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("%s must be a string", key)
	}
	s = strings.TrimSpace(s)
	if required && s == "" {
		return "", fmt.Errorf("%s is required", key)
	}
	return s, nil
}

func argInt(args map[string]any, key string, def, min, max int) int {
	if f, ok := args[key].(float64); ok {
		n := int(f)
		if n < min {
			return min
		}
		if n > max {
			return max
		}
		return n
	}
	return def
}

func argBool(args map[string]any, key string) bool {
	b, _ := args[key].(bool)
	return b
}

// schema builds a JSON Schema object for tool input.
func schema(props map[string]any, required ...string) map[string]any {
	s := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func str(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
func num(desc string, min, max int) map[string]any {
	return map[string]any{"type": "integer", "description": desc, "minimum": min, "maximum": max}
}
func boolean(desc string) map[string]any {
	return map[string]any{"type": "boolean", "description": desc}
}
func enum(desc string, vals ...string) map[string]any {
	return map[string]any{"type": "string", "description": desc, "enum": vals}
}

// ---- rate limiting ----

type rateLimiter struct {
	mu     sync.Mutex
	n      int
	window time.Duration
	hits   map[string][]time.Time
}

func newRateLimiter(n int, window time.Duration) *rateLimiter {
	return &rateLimiter{n: n, window: window, hits: map[string][]time.Time{}}
}

func (l *rateLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	h := l.hits[key]
	i := 0
	for i < len(h) && now.Sub(h[i]) > l.window {
		i++
	}
	h = h[i:]
	if len(h) >= l.n {
		l.hits[key] = h
		return false
	}
	l.hits[key] = append(h, now)
	return true
}
