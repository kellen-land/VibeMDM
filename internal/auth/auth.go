// Package auth handles console authentication: passwords, sessions, API tokens.
package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/vibemdm/vibemdm/internal/model"
	"github.com/vibemdm/vibemdm/internal/store"
)

const (
	SessionCookie = "vibemdm_session"
	tokenPrefix   = "vmdm_"
)

// HashPassword hashes a password with bcrypt.
func HashPassword(pw string) (string, error) {
	if len(pw) < 8 {
		return "", errors.New("password must be at least 8 characters")
	}
	b, err := bcrypt.GenerateFromPassword([]byte(pw), 12)
	return string(b), err
}

// CheckPassword verifies a password.
func CheckPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

// RandomToken returns a URL-safe random string.
func RandomToken(nBytes int) string {
	b := make([]byte, nBytes)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// NewAPIToken returns (plaintext, prefix).
func NewAPIToken() (string, string) {
	t := tokenPrefix + RandomToken(32)
	return t, t[:12]
}

// Principal is the authenticated caller.
type Principal struct {
	UserID   string
	Username string
	Role     string // admin | viewer
	Via      string // session | token | agent
	// SessionHash identifies the session (Via == "session").
	SessionHash string
	// MustChangePassword restricts the session to changing the password.
	MustChangePassword bool
}

// IsAdmin reports admin role.
func (p *Principal) IsAdmin() bool { return p != nil && p.Role == "admin" }

type ctxKey struct{}

// FromContext returns the principal.
func FromContext(ctx context.Context) *Principal {
	p, _ := ctx.Value(ctxKey{}).(*Principal)
	return p
}

// WithPrincipal stores the principal.
func WithPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, ctxKey{}, p)
}

// Authenticator resolves sessions and tokens.
type Authenticator struct {
	Store         *store.Store
	SecureCookies bool
	SessionTTL    time.Duration

	limiter *loginLimiter
}

// New creates an Authenticator.
func New(st *store.Store, secure bool, ttl time.Duration) *Authenticator {
	return &Authenticator{Store: st, SecureCookies: secure, SessionTTL: ttl, limiter: newLoginLimiter()}
}

// dummyHash gives unknown usernames the same bcrypt cost as real ones (no timing oracle).
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("vibemdm-timing-equalizer"), 12)

// ErrRateLimited is returned when login attempts are throttled.
var ErrRateLimited = errors.New("too many failed attempts; try again later")

// Login validates credentials and creates a session cookie. Attempts are
// throttled per client IP and per username (with growing lockouts).
func (a *Authenticator) Login(w http.ResponseWriter, r *http.Request, username, password string) (*model.AdminUser, error) {
	ip := clientIP(r)
	ukey := "u:" + strings.ToLower(strings.TrimSpace(username))
	if !a.limiter.allow(ip) || !a.limiter.allow(ukey) {
		return nil, ErrRateLimited
	}
	user, hash, err := a.Store.GetUserByUsername(r.Context(), username)
	if err != nil {
		bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
	}
	if err != nil || !user.Enabled || !CheckPassword(hash, password) {
		a.limiter.fail(ip)
		a.limiter.fail(ukey)
		return nil, errors.New("invalid username or password")
	}
	a.limiter.reset(ip)
	a.limiter.reset(ukey)
	if err := a.startSession(w, r, user.ID); err != nil {
		return nil, err
	}
	a.Store.TouchUserLogin(r.Context(), user.ID)
	return user, nil
}

func (a *Authenticator) startSession(w http.ResponseWriter, r *http.Request, userID string) error {
	tok := RandomToken(32)
	if err := a.Store.CreateSession(r.Context(), store.HashToken(tok), userID, a.SessionTTL, r.UserAgent(), clientIP(r)); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name: SessionCookie, Value: tok, Path: "/", HttpOnly: true, Secure: a.SecureCookies,
		SameSite: http.SameSiteLaxMode, Expires: time.Now().Add(a.SessionTTL),
	})
	return nil
}

// RotateSession ends every session of the user (including the current one) and
// issues a fresh cookie; used after a password change.
func (a *Authenticator) RotateSession(w http.ResponseWriter, r *http.Request, userID string) error {
	if err := a.Store.DeleteUserSessions(r.Context(), userID, ""); err != nil {
		return err
	}
	return a.startSession(w, r, userID)
}

// Logout destroys the session.
func (a *Authenticator) Logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(SessionCookie); err == nil {
		a.Store.DeleteSession(r.Context(), store.HashToken(c.Value))
	}
	http.SetCookie(w, &http.Cookie{Name: SessionCookie, Value: "", Path: "/", HttpOnly: true, Secure: a.SecureCookies, MaxAge: -1})
}

// Resolve identifies the caller from cookie or bearer token.
func (a *Authenticator) Resolve(r *http.Request) *Principal {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		tok := strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
		if strings.HasPrefix(tok, tokenPrefix) {
			t, err := a.Store.GetAPIToken(r.Context(), store.HashToken(tok))
			if err == nil {
				return &Principal{UserID: t.ID, Username: "token:" + t.Name, Role: t.Role, Via: "token"}
			}
		}
		return nil
	}
	if c, err := r.Cookie(SessionCookie); err == nil && c.Value != "" {
		h := store.HashToken(c.Value)
		u, err := a.Store.GetSessionUser(r.Context(), h)
		if err == nil {
			return &Principal{UserID: u.ID, Username: u.Username, Role: u.Role, Via: "session", SessionHash: h, MustChangePassword: u.MustChangePassword}
		}
	}
	return nil
}

// Require is middleware that enforces authentication (and admin role if adminOnly).
func (a *Authenticator) Require(adminOnly bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := a.Resolve(r)
		if p == nil {
			writeJSONError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		if p.MustChangePassword && r.URL.Path != "/api/v1/auth/password" {
			writeJSONError(w, http.StatusForbidden, "password change required")
			return
		}
		if adminOnly && !p.IsAdmin() {
			writeJSONError(w, http.StatusForbidden, "admin role required")
			return
		}
		// Mutating requests via session cookie must come from our own origin (CSRF).
		if p.Via == "session" && r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
			if !sameOrigin(r) {
				writeJSONError(w, http.StatusForbidden, "cross-origin request rejected")
				return
			}
		}
		next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), p)))
	})
}

// RequireWrite rejects viewers for non-GET requests.
func RequireWrite(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := FromContext(r.Context())
		if r.Method != http.MethodGet && r.Method != http.MethodHead && !p.IsAdmin() {
			writeJSONError(w, http.StatusForbidden, "admin role required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// SameOrigin reports whether a browser request came from this site (CSRF check).
func SameOrigin(r *http.Request) bool { return sameOrigin(r) }

func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		origin = r.Header.Get("Referer")
	}
	if origin == "" {
		// Non-browser clients (curl) using cookies: accept when a custom header is set.
		return r.Header.Get("X-Requested-With") != "" || r.Header.Get("Sec-Fetch-Site") == "same-origin" || r.Header.Get("Sec-Fetch-Site") == ""
	}
	host := r.Host
	o := strings.TrimPrefix(strings.TrimPrefix(origin, "https://"), "http://")
	if i := strings.Index(o, "/"); i >= 0 {
		o = o[:i]
	}
	return strings.EqualFold(o, host)
}

var trustedProxies []netip.Prefix

// SetTrustedProxies configures which peers may set X-Forwarded-For (CIDRs or IPs).
// With none configured, forwarding headers are ignored.
func SetTrustedProxies(list []string) error {
	var out []netip.Prefix
	for _, s := range list {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if !strings.Contains(s, "/") {
			a, err := netip.ParseAddr(s)
			if err != nil {
				return fmt.Errorf("trusted proxy %q: %w", s, err)
			}
			out = append(out, netip.PrefixFrom(a, a.BitLen()))
			continue
		}
		p, err := netip.ParsePrefix(s)
		if err != nil {
			return fmt.Errorf("trusted proxy %q: %w", s, err)
		}
		out = append(out, p.Masked())
	}
	trustedProxies = out
	return nil
}

func isTrustedProxy(a netip.Addr) bool {
	a = a.Unmap()
	for _, p := range trustedProxies {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// clientIP returns the caller's address. X-Forwarded-For is honoured only when
// the direct peer is a trusted proxy, and is read right-to-left so a client
// cannot prepend a spoofed address.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer, err := netip.ParseAddr(host)
	if err != nil || !isTrustedProxy(peer) {
		return host
	}
	parts := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	for i := len(parts) - 1; i >= 0; i-- {
		ip, err := netip.ParseAddr(strings.TrimSpace(parts[i]))
		if err != nil {
			break
		}
		if !isTrustedProxy(ip) {
			return ip.Unmap().String()
		}
	}
	// X-Real-IP is not consulted: a client that reaches the published port directly (from an
	// address inside the trusted range, such as the Docker bridge gateway) could set it freely.
	return host
}

// ClientIP exposes the client IP helper.
func ClientIP(r *http.Request) string { return clientIP(r) }

func writeJSONError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// ---- login rate limiting ----

type loginLimiter struct {
	mu    sync.Mutex
	fails map[string]*failRec
}

type failRec struct {
	count int
	until time.Time
}

func newLoginLimiter() *loginLimiter {
	l := &loginLimiter{fails: map[string]*failRec{}}
	go func() {
		for range time.Tick(5 * time.Minute) {
			l.mu.Lock()
			for k, v := range l.fails {
				if time.Now().After(v.until.Add(2 * time.Hour)) {
					delete(l.fails, k)
				}
			}
			l.mu.Unlock()
		}
	}()
	return l
}

const loginBurst = 6 // failures allowed before a lockout

func (l *loginLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	rec := l.fails[key]
	return rec == nil || time.Now().After(rec.until)
}

// fail records a failure; every loginBurst failures lock the key out for
// 1, 5, 15, then 60 minutes.
func (l *loginLimiter) fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	rec := l.fails[key]
	if rec == nil {
		rec = &failRec{}
		l.fails[key] = rec
	}
	rec.count++
	if rec.count%loginBurst == 0 {
		steps := []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute, time.Hour}
		n := rec.count/loginBurst - 1
		if n >= len(steps) {
			n = len(steps) - 1
		}
		rec.until = time.Now().Add(steps[n])
	}
}

func (l *loginLimiter) reset(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.fails, ip)
}
