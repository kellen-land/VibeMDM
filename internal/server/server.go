// Package server wires VibeMDM together: NanoMDM, SCEP, agent API, admin API, web UI.
package server

import (
	"bufio"
	"context"
	"crypto/subtle"
	"database/sql"
	"errors"
	"fmt"
	"github.com/vibemdm/vibemdm/internal/agentinstall"
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
	"github.com/vibemdm/vibemdm/internal/commands"
	"github.com/vibemdm/vibemdm/internal/compliance"
	"github.com/vibemdm/vibemdm/internal/config"
	"github.com/vibemdm/vibemdm/internal/db"
	"github.com/vibemdm/vibemdm/internal/ddmsvc"
	"github.com/vibemdm/vibemdm/internal/enroll"
	"github.com/vibemdm/vibemdm/internal/escrow"
	"github.com/vibemdm/vibemdm/internal/health"
	"github.com/vibemdm/vibemdm/internal/logsink"
	"github.com/vibemdm/vibemdm/internal/mcp"
	"github.com/vibemdm/vibemdm/internal/mdmsvc"
	"github.com/vibemdm/vibemdm/internal/onboarding"
	"github.com/vibemdm/vibemdm/internal/policy"
	"github.com/vibemdm/vibemdm/internal/scepsrv"
	"github.com/vibemdm/vibemdm/internal/secrets"
	"github.com/vibemdm/vibemdm/internal/shell"
	"github.com/vibemdm/vibemdm/internal/store"
	"github.com/vibemdm/vibemdm/internal/superwf"
	"github.com/vibemdm/vibemdm/internal/webhook"
	"github.com/vibemdm/vibemdm/internal/webui"
)

// Server is the assembled application.
type Server struct {
	cfg        *config.Config
	log        *slog.Logger
	sqlDB      *sql.DB
	st         *store.Store
	scep       *scepsrv.Server
	hooks      *webhook.Dispatcher
	health     *health.Engine
	superSvc   *superwf.Service
	onboarding *onboarding.Service
	compliance *compliance.Service
	escrow     *escrow.Identity
	cmdr       *mdmsvc.Commander
	sink       *logsink.Sink
	baseLog    *slog.Logger
	mux        *http.ServeMux
	http       *http.Server
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
	// Tee every log record at/above VIBEMDM_LOG_DB_LEVEL into PostgreSQL.
	baseLogger := logger
	sink := logsink.New(sqlDB, cfg.LogDBLevel)
	logger = slog.New(logsink.Wrap(logger.Handler(), sink))
	slog.SetDefault(logger)

	st := store.New(sqlDB)
	box, err := secrets.New(cfg.EncryptionKey)
	if err != nil {
		return nil, err
	}
	st.Box = box
	// Fail fast before anything reads (or could regenerate) a secret.
	if err := st.VerifyEncryptionKey(ctx); err != nil {
		return nil, err
	}
	if box.Enabled() {
		n, err := st.ResealSecrets(ctx)
		if err != nil {
			return nil, fmt.Errorf("sealing stored secrets: %w", err)
		}
		if n > 0 {
			logger.Info("encrypted previously plaintext secrets at rest", "count", n)
		}
	} else {
		logger.Warn("VIBEMDM_ENCRYPTION_KEY is not set; private keys and FileVault recovery keys are stored unencrypted. Set a long random value to enable at-rest encryption.")
	}
	s := &Server{cfg: cfg, log: logger, baseLog: baseLogger, sink: sink, sqlDB: sqlDB, st: st, mux: http.NewServeMux()}

	if err := s.bootstrap(ctx); err != nil {
		return nil, err
	}
	settings, _ := st.GetSettings(ctx)
	for _, d := range []string{"packages", "tmp"} {
		os.MkdirAll(filepath.Join(cfg.DataDir, d), 0o755)
	}

	// SCEP CA
	// SCEP accepts the static challenge (admin-only, shown in Settings → SCEP) or a
	// minted one embedded in a downloaded enrollment profile.
	scepServer, err := scepsrv.New(ctx, st, func(ctx context.Context, challenge string) bool {
		if se, err := st.GetSettings(ctx); err == nil && se.SCEPChallenge != "" &&
			subtle.ConstantTimeCompare([]byte(se.SCEPChallenge), []byte(challenge)) == 1 {
			return true
		}
		return st.ConsumeSCEPChallenge(ctx, challenge)
	}, settings.OrgName, logger.With("component", "scep"))
	if err != nil {
		return nil, fmt.Errorf("scep: %w", err)
	}
	s.scep = scepServer

	// NanoMDM storage sharing our database.
	nlog := newNanoLogger(logger.With("component", "nanomdm"))
	pgStorage, err := pgsql.New(pgsql.WithDB(sqlDB), pgsql.WithLogger(nlog.With("storage", "pgsql")))
	if err != nil {
		return nil, fmt.Errorf("nanomdm storage: %w", err)
	}
	// APNs keys are sealed at rest; everything else is NanoMDM's own storage.
	mdmStorage := &sealedPushStore{PgSQLStorage: pgStorage, st: st}

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
	s.cmdr = commander

	// FileVault escrow identity (load, or create on first boot; never replace on read errors).
	escrowID, err := s.loadEscrowIdentity(ctx, settings.OrgName)
	if err != nil {
		return nil, fmt.Errorf("escrow identity: %w", err)
	}
	s.escrow = escrowID

	// Core MDM service chain: certauth -> vibemdm hooks -> nanomdm core.
	core := nanomdmsvc.New(mdmStorage,
		nanomdmsvc.WithLogger(nlog.With("service", "nanomdm")),
		nanomdmsvc.WithUserAuthenticate(nanomdmsvc.NewUAService(mdmStorage, false)),
		nanomdmsvc.WithGetToken(nanomdmsvc.NewTokenMux()),
		nanomdmsvc.WithDeclarativeManagement(ddmsvc.New(st, logger.With("component", "ddm"))),
	)
	agentInstaller := agentinstall.New(st, commander, cfg.DataDir, logger.With("component", "agentinstall"))
	agentInstaller.Register(s.mux)
	var mdmService service.CheckinAndCommandService = mdmsvc.New(st, core, commander, hooks, logger.With("component", "mdm")).WithEscrow(escrowID).WithAgentInstall(agentInstaller)
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
	s.mux.HandleFunc("GET /enroll/configurator/{token}", s.configuratorProfileHandler)

	// Agent API
	notifier := agentapi.NewNotifier()
	shellHub := shell.NewHub()
	engine := policy.New(st, logger.With("component", "policy"))
	agentHandler := agentapi.New(st, engine, notifier, shellHub, hooks, cfg.DataDir, cfg.AgentDir, cfg.Version, s.enrollmentProfile, logger.With("component", "agent"))
	s.compliance = compliance.NewService(st, notifier.Notify, logger.With("component", "compliance"))
	agentHandler.WithCompliance(s.compliance)
	s.superSvc = superwf.New(st, notifier.Notify, logger.With("component", "super"))
	s.onboarding = onboarding.New(st, engine, notifier.Notify, logger.With("component", "onboarding"))
	agentHandler.WithOnboarding(s.onboarding)
	agentHandler.OnAgentEnrolled = agentInstaller.AgentEnrolled
	agentHandler.Register(s.mux)

	// Health: misconfiguration detection, remediation and self-healing.
	s.health = health.NewEngine(health.Builtin(health.Deps{
		St: st, Cmdr: commander, EncryptionEnabled: box.Enabled(), Wake: notifier.Notify, AgentDir: cfg.AgentDir, Rescan: s.compliance.Rescan,
		InstallAgent: func(ctx context.Context, id string) error {
			c, err := st.GetComputer(ctx, id)
			if err != nil {
				return err
			}
			return agentInstaller.Install(ctx, c, "health")
		},
	}), st, hooks, logger.With("component", "health"))

	// Admin API
	if err := auth.SetTrustedProxies(cfg.TrustedProxies); err != nil {
		return nil, err
	}
	if len(cfg.TrustedProxies) == 0 {
		logger.Info("no VIBEMDM_TRUSTED_PROXIES configured; X-Forwarded-For is ignored and client IPs are the direct peer")
	}
	secure := strings.HasPrefix(settings.ServerURL, "https://") || strings.HasPrefix(cfg.ServerURL, "https://")
	authn := auth.New(st, secure, cfg.SessionTTL)
	adminAPI := api.New(api.Deps{
		Cfg: cfg, Store: st, Auth: authn, Commander: commander, Engine: engine, Notifier: notifier, SCEP: scepServer,
		PushStore: mdmStorage, Profile: s.enrollmentProfile, ShellHub: shellHub, Hooks: hooks, Escrow: escrowID, Health: s.health, Compliance: s.compliance, Super: s.superSvc, Onboarding: s.onboarding, AgentInstall: agentInstaller, Log: logger.With("component", "api"),
	})
	adminAPI.Register(s.mux)

	// MCP server for AI agents (off unless enabled in settings; API tokens only).
	s.mux.Handle("/mcp", mcp.New(st, authn, cfg.Version, logger.With("component", "mcp"), mcp.Tools(mcp.Deps{
		St: st, Cmdr: commander, Health: s.health, Compliance: s.compliance, Engine: engine, Notify: notifier.Notify,
	})))

	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := sqlDB.PingContext(r.Context()); err != nil {
			s.log.Warn("health check: database unreachable", "err", err)
			http.Error(w, "database unavailable", 503)
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
		Handler:           s.requestLogger(securityHeaders(s.mux, secure)),
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
			pw = auth.RandomToken(18)
			generated = true
		} else if err := checkInitialPassword(pw); err != nil {
			return err
		}
		hash, err := auth.HashPassword(pw)
		if err != nil {
			return fmt.Errorf("initial admin password: %w", err)
		}
		u, err := s.st.CreateUser(ctx, "admin", "Administrator", "", "admin", hash)
		if err != nil {
			return err
		}
		// Whatever the source, the bootstrap password must be replaced at first login.
		s.st.SetMustChangePassword(ctx, u.ID, true)
		if generated {
			// Never put the password in the searchable log table: write it to a 0600 file
			// and print it to the console only (baseLog bypasses the DB log sink).
			path := filepath.Join(s.cfg.DataDir, "initial-admin-password")
			os.MkdirAll(s.cfg.DataDir, 0o755)
			if err := os.WriteFile(path, []byte(pw+"\n"), 0o600); err != nil {
				return fmt.Errorf("writing initial admin password: %w", err)
			}
			s.baseLog.Warn("created the initial admin user with a generated password; it must be changed at first login",
				"username", "admin", "password_file", path)
			fmt.Fprintf(os.Stderr, "\n  VibeMDM initial admin login:  admin / %s\n  (also saved to %s; you will be asked to change it)\n\n", pw, path)
		} else {
			s.log.Info("created initial admin user from VIBEMDM_ADMIN_PASSWORD; it must be changed at first login", "username", "admin")
		}
	}
	return nil
}

// checkInitialPassword rejects placeholder or weak bootstrap passwords from the environment.
func checkInitialPassword(pw string) error {
	switch strings.ToLower(pw) {
	case "change-me-please", "change-me", "changeme", "admin", "password", "vibemdm", "adminadmin":
		return errors.New("VIBEMDM_ADMIN_PASSWORD is a placeholder value; set a unique password or leave it empty to generate one")
	}
	if len(pw) < 12 {
		return errors.New("VIBEMDM_ADMIN_PASSWORD must be at least 12 characters (or leave it empty to generate one)")
	}
	return nil
}

// logMaintenance archives yesterday's logs (when enabled) and prunes old ones.
func (s *Server) logMaintenance(ctx context.Context) {
	settings, err := s.st.GetSettings(ctx)
	if err != nil {
		return
	}
	if settings.LogArchiveEnabled {
		dir := filepath.Join(s.cfg.DataDir, "log-archive")
		yesterday := time.Now().UTC().Add(-24 * time.Hour)
		for _, src := range []string{"server", "agent"} {
			path, n, err := s.st.ArchiveLogDay(ctx, src, yesterday, dir)
			if err != nil {
				s.log.Warn("archiving logs", "source", src, "err", err)
			} else if n > 0 {
				s.log.Info("archived logs", "source", src, "entries", n, "file", filepath.Base(path))
			}
		}
	}
	s.st.PruneLogs(ctx, settings.LogRetentionDays)
	if d := s.sink.Dropped(); d > 0 {
		s.log.Warn("log records dropped because the database could not keep up", "total_dropped", d)
	}
}

// filevaultKeySweep asks Macs with FileVault on but no escrowed key for
// SecurityInfo (at most every 4 hours each) so newly generated keys — e.g. after
// Escrow Buddy runs at login — are collected promptly.
func (s *Server) filevaultKeySweep(ctx context.Context) {
	comps, err := s.st.ComputersNeedingKeyCheck(ctx, 4*time.Hour)
	if err != nil {
		s.log.Warn("filevault sweep", "err", err)
		return
	}
	for i := range comps {
		cmd, err := commands.SecurityInfo()
		if err != nil {
			return
		}
		if _, err := s.cmdr.Enqueue(ctx, &comps[i], cmd, "filevault_check", "", "system"); err != nil {
			s.log.Debug("filevault sweep enqueue", "computer", comps[i].Name, "err", err)
		}
	}
	if len(comps) > 0 {
		s.log.Info("requested SecurityInfo from Macs missing an escrowed FileVault key", "count", len(comps))
	}
}

// mobileInventorySweep asks iPhones and iPads for fresh inventory once their last report is older
// than the inventory interval. Mobile devices have no agent, so the server has to ask.
func (s *Server) mobileInventorySweep(ctx context.Context) {
	settings, err := s.st.GetSettings(ctx)
	if err != nil {
		return
	}
	hours := settings.InventoryIntervalHours
	if hours < 1 {
		hours = 24
	}
	devs, err := s.st.MobileDevicesNeedingInventory(ctx, time.Duration(hours)*time.Hour, 200)
	if err != nil {
		s.log.Warn("mobile inventory sweep", "err", err)
		return
	}
	for i := range devs {
		if err := s.cmdr.RequestMobileInventory(ctx, &devs[i], "system"); err != nil {
			s.log.Debug("mobile inventory sweep enqueue", "mobile_device", devs[i].Name, "err", err)
		}
	}
	if len(devs) > 0 {
		s.log.Info("requested inventory from mobile devices", "count", len(devs))
	}
}

// loadEscrowIdentity returns the FileVault escrow identity, creating it only
// when none is stored. A stored identity that cannot be read is fatal: replacing
// it would make every previously escrowed key undecryptable.
func (s *Server) loadEscrowIdentity(ctx context.Context, orgName string) (*escrow.Identity, error) {
	certPEM, keyPEM, err := s.st.GetEscrowIdentity(ctx)
	if err == nil {
		return escrow.ParseIdentity(certPEM, keyPEM)
	}
	if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	id, certPEM, keyPEM, err := escrow.NewIdentity(orgName)
	if err != nil {
		return nil, err
	}
	if err := s.st.CreateEscrowIdentity(ctx, certPEM, keyPEM); err != nil {
		return nil, err
	}
	s.log.Info("created FileVault escrow identity")
	return id, nil
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
	purpose := enroll.PurposeFrom(ctx)
	ttl, single := purpose.ChallengePolicy()
	challenge, err := s.st.MintSCEPChallenge(ctx, string(purpose), ttl, single)
	if err != nil {
		return nil, err
	}
	return enroll.Build(enroll.Params{
		ServerURL: settings.ServerURL, Topic: topics[0], SCEPChallenge: challenge, OrgName: settings.OrgName,
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

// redactPath hides secrets that travel in URL paths.
func redactPath(p string) string {
	if strings.HasPrefix(p, "/enroll/configurator/") {
		return "/enroll/configurator/[redacted]"
	}
	return p
}

// securityHeaders sets browser hardening headers on every response.
func securityHeaders(next http.Handler, https bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; "+
			"font-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'; object-src 'none'")
		if https {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			h.Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(sw, r)
		dur := time.Since(start)
		// Successful console, asset and agent requests are routine (the agent logs what they did
		// itself) and stay at debug; errors, MDM, SCEP and enrollment traffic are logged at info.
		lvl := slog.LevelDebug
		routine := strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/assets/") ||
			(strings.HasPrefix(r.URL.Path, "/agent/v1/") && r.URL.Path != "/agent/v1/enroll")
		if sw.status >= 500 {
			lvl = slog.LevelWarn
		} else if sw.status >= 400 || !routine {
			lvl = slog.LevelInfo
		}
		s.log.Log(r.Context(), lvl, fmt.Sprintf("%s %s → %d", r.Method, redactPath(r.URL.Path), sw.status), "dur", dur, "remote", auth.ClientIP(r), "component", "http")
	})
}

// Run starts background jobs and serves until ctx is cancelled.
func (s *Server) Run(ctx context.Context) error {
	s.hooks.Start(ctx)
	var lastSinkErr time.Time
	s.sink.Start(ctx, func(err error) {
		// Report sink failures through the plain logger (never back into the sink), at most once a minute.
		if time.Since(lastSinkErr) > time.Minute {
			lastSinkErr = time.Now()
			s.baseLog.Warn("writing logs to the database failed", "err", err)
		}
	})
	go s.housekeeping(ctx)
	go func() {
		// First health evaluation shortly after start-up, so the console never says "not checked yet" for long.
		select {
		case <-ctx.Done():
		case <-time.After(20 * time.Second):
			s.health.RunAll(ctx)
		}
	}()
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
	tick := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			tick++
			if tick%10 == 1 {
				s.filevaultKeySweep(ctx)
				s.st.LAPSExpireTokenGrants(ctx)
				s.mobileInventorySweep(ctx)
			}
			if tick%30 == 1 {
				if n, err := s.cmdr.ReconcileDDM(ctx); err != nil {
					s.log.Warn("DDM reconcile", "err", err)
				} else if n > 0 {
					s.log.Info("asked Macs to sync declarations", "count", n)
				}
			}
			if tick%10 == 3 {
				s.compliance.Schedule(ctx)
			}
			if tick%30 == 4 {
				s.superSvc.Reconcile(ctx)
			}
			if tick%5 == 2 {
				// Offset from the other jobs so the first run sees fresh state.
				s.health.RunAll(ctx)
			}
			if tick%60 == 1 {
				s.st.PruneSCEPChallenges(ctx)
				s.st.PruneHealth(ctx, 30)
				s.st.PruneCommandEvents(ctx, 90)
				s.logMaintenance(ctx)
			}
			s.st.ExpireStalePolicyLogs(ctx)
			s.st.ExpireStaleTasks(ctx)
			s.st.ExpireStaleShellSessions(ctx)
			s.st.ExpireStaleAppInstalls(ctx)
			if tick%60 == 1 {
				s.st.ExpireComplianceScans(ctx)
			}
			s.st.DeleteExpiredSessions(ctx)
		}
	}
}
