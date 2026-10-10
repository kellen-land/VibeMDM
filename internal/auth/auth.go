// Package auth handles console authentication: passwords, sessions, API tokens.
package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"net/http"
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

// Login validates credentials and creates a session cookie.
func (a *Authenticator) Login(w http.ResponseWriter, r *http.Request, username, password string) (*model.AdminUser, error) {
	ip := clientIP(r)
	if !a.limiter.allow(ip) {
		return nil, errors.New("too many failed attempts; try again in a minute")
	}
	user, hash, err := a.Store.GetUserByUsername(r.Context(), username)
	if err != nil || !user.Enabled || !CheckPassword(hash, password) {
		a.limiter.fail(ip)
		// constant-ish response
		time.Sleep(150 * time.Millisecond)
		return nil, errors.New("invalid username or password")
	}
	a.limiter.reset(ip)
	tok := RandomToken(32)
	if err := a.Store.CreateSession(r.Context(), store.HashToken(tok), user.ID, a.SessionTTL, r.UserAgent(), ip); err != nil {
		return nil, err
	}
	a.Store.TouchUserLogin(r.Context(), user.ID)
	http.SetCookie(w, &http.Cookie{
		Name: SessionCookie, Value: tok, Path: "/", HttpOnly: true, Secure: a.SecureCookies,
		SameSite: http.SameSiteLaxMode, Expires: time.Now().Add(a.SessionTTL),
	})
	return user, nil
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
		u, err := a.Store.GetSessionUser(r.Context(), store.HashToken(c.Value))
		if err == nil {
			return &Principal{UserID: u.ID, Username: u.Username, Role: u.Role, Via: "session"}
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

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	if xr := r.Header.Get("X-Real-IP"); xr != "" {
		return xr
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
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
				if time.Now().After(v.until.Add(10 * time.Minute)) {
					delete(l.fails, k)
				}
			}
			l.mu.Unlock()
		}
	}()
	return l
}

func (l *loginLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	rec := l.fails[ip]
	return rec == nil || rec.count < 8 || time.Now().After(rec.until)
}

func (l *loginLimiter) fail(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	rec := l.fails[ip]
	if rec == nil {
		rec = &failRec{}
		l.fails[ip] = rec
	}
	if time.Now().After(rec.until) && rec.count >= 8 {
		rec.count = 0
	}
	rec.count++
	if rec.count >= 8 {
		rec.until = time.Now().Add(time.Minute)
	}
}

func (l *loginLimiter) reset(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.fails, ip)
}
