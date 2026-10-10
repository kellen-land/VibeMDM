// Package server wires VibeMDM together: NanoMDM, SCEP, agent API, admin API, web UI.
package server

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/micromdm/nanomdm/cryptoutil"
	httpmdm "github.com/micromdm/nanomdm/http/mdm"
	"github.com/micromdm/nanomdm/push/nanopush"
	pushsvc "github.com/micromdm/nanomdm/push/service"
	"github.com/micromdm/nanomdm/service"
	"github.com/micromdm/nanomdm/service/certauth"
	"github.com/micromdm/nanomdm/service/dump"
	nanomdmsvc "github.com/micromdm/nanomdm/service/nanomdm"
	"github.com/micromdm/nanomdm/storage/pgsql"

	"github.com/vibemdm/vibemdm/internal/agentapi"
	"github.com/vibemdm/vibemdm/internal/api"
	"github.com/vibemdm/vibemdm/internal/auth"
	"github.com/vibemdm/vibemdm/internal/config"
	"github.com/vibemdm/vibemdm/internal/db"
	"github.com/vibemdm/vibemdm/internal/enroll"
	"github.com/vibemdm/vibemdm/internal/mdmsvc"
	"github.com/vibemdm/vibemdm/internal/policy"
	"github.com/vibemdm/vibemdm/internal/scepsrv"
	"github.com/vibemdm/vibemdm/internal/shell"
	"github.com/vibemdm/vibemdm/internal/store"
	"github.com/vibemdm/vibemdm/internal/webhook"
	"github.com/vibemdm/vibemdm/internal/webui"
)

// Server is the assembled application.
type Server struct {
	cfg   *config.Config
	log   *slog.Logger
	sqlDB *sql.DB
	st    *store.Store
	scep  *scepsrv.Server
	hooks *webhook.Dispatcher
	mux   *http.ServeMux
	http  *http.Server
}

// New builds the server (connects DB, migrates, wires services).
func New(ctx context.Context, cfg *config.Config, logger *slog.Logger) (*Server, error) {
	sqlDB, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	ran, err := db.Migrate(ctx, sqlDB)
	if err != nil {
		return nil, fmt.Errorf("migrations: %w", err)
	}
	if len(ran) > 0 {
		logger.Info("applied database migrations", "versions", ran)
	}
	st := store.New(sqlDB)
	s := &Server{cfg: cfg, log: logger, sqlDB: sqlDB, st: st, mux: http.NewServeMux()}

	if err := s.bootstrap(ctx); err != nil {
		return nil, err
	}
	settings, _ := st.GetSettings(ctx)
	for _, d := range []string{"packages", "tmp"} {
		os.MkdirAll(filepath.Join(cfg.DataDir, d), 0o755)
	}

	// SCEP CA
	scepServer, err := scepsrv.New(ctx, st, func(ctx context.Context) string {
		se, err := st.GetSettings(ctx)
		if err != nil {
			return ""
		}
		return se.SCEPChallenge
	}, settings.OrgName, logger.With("component", "scep"))
	if err != nil {
		return nil, fmt.Errorf("scep: %w", err)
	}
	s.scep = scepServer

	// NanoMDM storage sharing our database.
	nlog := newNanoLogger(logger.With("component", "nanomdm"))
	mdmStorage, err := pgsql.New(pgsql.WithDB(sqlDB), pgsql.WithLogger(nlog.With("storage", "pgsql")))
	if err != nil {
		return nil, fmt.Errorf("nanomdm storage: %w", err)
	}

	// Push
	// APNs provider. nanopush v0.9 has no URL override, so an alternate push
	// server (for testing) is handled by pointing the HTTP client at it.
	pushOpts := []nanopush.Option{nanopush.WithWorkers(4)}
	if cfg.APNsURL != "" {
		logger.Info("using custom APNs server URL", "url", cfg.APNsURL)
		pushOpts = append(pushOpts, nanopush.WithNewClient(apnsClientFor(cfg.APNsURL)))
	}
	pushService := pushsvc.New(mdmStorage, mdmStorage, nanopush.NewFactory(pushOpts...), nlog.With("service", "push"))

	commander, err := mdmsvc.NewCommander(st, mdmStorage, pushService, logger.With("component", "commander"))
	if err != nil {
		return nil, err
	}

	hooks := webhook.New(sqlDB, logger.With("component", "webhook"))
	s.hooks = hooks


	// Core MDM service chain: certauth -> vibemdm hooks -> nanomdm core.
	core := nanomdmsvc.New(mdmStorage,
		nanomdmsvc.WithLogger(nlog.With("service", "nanomdm")),
		nanomdmsvc.WithUserAuthenticate(nanomdmsvc.NewUAService(mdmStorage, false)),
		nanomdmsvc.WithGetToken(nanomdmsvc.NewTokenMux()),
	)
	var mdmService service.CheckinAndCommandService = mdmsvc.New(st, core, commander, hooks, logger.With("component", "mdm"))
	mdmService = certauth.New(mdmService, mdmStorage, certauth.WithLogger(nlog.With("service", "certauth")))
	if cfg.DumpMDM {
		mdmService = dump.New(mdmService, os.Stdout)
	}

	sigOpts := []httpmdm.SigLogOption{httpmdm.SigLogWithLogger(nlog.With("handler", "cert-extract"))}
	if cfg.Debug {
		sigOpts = append(sigOpts, httpmdm.SigLogWithLogErrors(true))
	}
	var mdmHandler http.Handler = httpmdm.CheckinAndCommandHandler(mdmService, nlog.With("handler", "checkin-command"))
	mdmHandler = httpmdm.CertVerifyMiddleware(mdmHandler, scepServer, nlog.With("handler", "cert-verify"))
	mdmHandler = httpmdm.CertExtractMdmSignatureMiddleware(mdmHandler, httpmdm.MdmSignatureVerifierFunc(cryptoutil.VerifyMdmSignature), sigOpts...)
	s.mux.Handle("/mdm", mdmHandler)
	s.mux.Handle("/scep", scepServer)

	// Enrollment page & profile
	s.mux.HandleFunc("GET /enroll", s.enrollHandler)
	s.mux.HandleFunc("GET /enroll/profile", s.enrollProfileHandler)

	// Agent API
	notifier := agentapi.NewNotifier()
	shellHub := shell.NewHub()
	engine := policy.New(st, logger.With("component", "policy"))
	agentHandler := agentapi.New(st, engine, notifier, shellHub, hooks, cfg.DataDir, cfg.AgentDir, cfg.Version, s.enrollmentProfile, logger.With("component", "agent"))
	agentHandler.Register(s.mux)

	// Admin API
	secure := strings.HasPrefix(settings.ServerURL, "https://") || strings.HasPrefix(cfg.ServerURL, "https://")
	authn := auth.New(st, secure, cfg.SessionTTL)
	adminAPI := api.New(api.Deps{
		Cfg: cfg, Store: st, Auth: authn, Commander: commander, Engine: engine, Notifier: notifier, SCEP: scepServer,
		PushStore: mdmStorage, Profile: s.enrollmentProfile, ShellHub: shellHub, Hooks: hooks, Log: logger.With("component", "api"),
	})
	adminAPI.Register(s.mux)

	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := sqlDB.PingContext(r.Context()); err != nil {
			http.Error(w, "db: "+err.Error(), 503)
			return
		}
		w.Write([]byte("ok"))
	})
	s.mux.HandleFunc("GET /version", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"version":%q}`, cfg.Version)
	})
	s.mux.Handle("/", webui.Handler())

	s.http = &http.Server{
		Addr:              cfg.Listen,
		Handler:           s.requestLogger(s.mux),
		ReadHeaderTimeout: 15 * time.Second,
		ReadTimeout:       10 * time.Minute, // large package uploads
		WriteTimeout:      10 * time.Minute,
		IdleTimeout:       120 * time.Second,
	}
	return s, nil
}

// bootstrap ensures an admin user and generated secrets exist.
func (s *Server) bootstrap(ctx context.Context) error {
	settings, err := s.st.GetSettings(ctx)
	if err != nil {
		return err
	}
	changed := false
	if settings.ServerURL == "" && s.cfg.ServerURL != "" {
		settings.ServerURL = s.cfg.ServerURL
		changed = true
	}
	if settings.AgentEnrollmentSecret == "" {
		settings.AgentEnrollmentSecret = auth.RandomToken(24)
		changed = true
	}
	if settings.SCEPChallenge == "" {
		settings.SCEPChallenge = auth.RandomToken(24)
		changed = true
	}
	if changed {
		if err := s.st.SaveSettings(ctx, settings); err != nil {
			return err
		}
	}
	n, err := s.st.CountUsers(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		pw := s.cfg.AdminPassword
		generated := false
		if pw == "" {
			pw = auth.RandomToken(12)
			generated = true
		}
		hash, err := auth.HashPassword(pw)
		if err != nil {
			return fmt.Errorf("initial admin password: %w", err)
		}
		if _, err := s.st.CreateUser(ctx, "admin", "Administrator", "", "admin", hash); err != nil {
			return err
		}
		if generated {
			s.log.Warn("created initial admin user with a GENERATED password — change it after logging in", "username", "admin", "password", pw)
		} else {
			s.log.Info("created initial admin user from VIBEMDM_ADMIN_PASSWORD", "username", "admin")
		}
	}
	return nil
}

// enrollmentProfile renders the current enrollment profile.
func (s *Server) enrollmentProfile(ctx context.Context) ([]byte, error) {
	settings, err := s.st.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	topics, err := s.st.GetPushCertTopics(ctx)
	if err != nil {
		return nil, err
	}
	if len(topics) == 0 {
		return nil, errors.New("no APNs push certificate has been uploaded")
	}
	return enroll.Build(enroll.Params{
		ServerURL: settings.ServerURL, Topic: topics[0], SCEPChallenge: settings.SCEPChallenge, OrgName: settings.OrgName,
		DisplayName: settings.EnrollmentProfileName, Identifier: settings.EnrollmentProfileIdentifier,
		SigningCertPEM: settings.SigningCertPEM, SigningKeyPEM: settings.SigningKeyPEM,
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap lets http.ResponseController reach the underlying writer (WebSocket hijack).
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// Hijack exposes the connection for WebSocket upgrades.
func (w *statusWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := w.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}

func (s *Server) requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(sw, r)
		dur := time.Since(start)
		lvl := slog.LevelDebug
		if sw.status >= 500 {
			lvl = slog.LevelWarn
		} else if sw.status >= 400 || (!strings.HasPrefix(r.URL.Path, "/api/") && !strings.HasPrefix(r.URL.Path, "/assets/") && !strings.HasPrefix(r.URL.Path, "/agent/v1/tasks")) {
			lvl = slog.LevelInfo
		}
		s.log.Log(r.Context(), lvl, "http", "method", r.Method, "path", r.URL.Path, "status", sw.status, "dur", dur.Round(time.Millisecond).String(), "remote", auth.ClientIP(r))
	})
}

// Run starts background jobs and serves until ctx is cancelled.
func (s *Server) Run(ctx context.Context) error {
	s.hooks.Start(ctx)
	go s.housekeeping(ctx)
	errCh := make(chan error, 1)
	go func() {
		s.log.Info("VibeMDM listening", "addr", s.cfg.Listen, "version", s.cfg.Version)
		if err := s.http.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	s.log.Info("shutting down")
	err := s.http.Shutdown(shutdownCtx)
	s.sqlDB.Close()
	return err
}

func (s *Server) housekeeping(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.st.ExpireStalePolicyLogs(ctx)
			s.st.ExpireStaleTasks(ctx)
			s.st.ExpireStaleShellSessions(ctx)
			s.st.ExpireStaleAppInstalls(ctx)
			s.st.DeleteExpiredSessions(ctx)
		}
	}
}
