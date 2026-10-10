// Command vibemdm is the VibeMDM server.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/vibemdm/vibemdm/internal/config"
	"github.com/vibemdm/vibemdm/internal/db"
	"github.com/vibemdm/vibemdm/internal/secrets"
	"github.com/vibemdm/vibemdm/internal/server"
	"github.com/vibemdm/vibemdm/internal/store"
)

// set via -ldflags "-X main.version=..."
var version = "dev"

const serverUsage = `VibeMDM server

Usage:
  vibemdm                          run the server (configured by VIBEMDM_* environment variables)
  vibemdm version                  print the version
  vibemdm rotate-encryption-key    re-encrypt stored secrets from one key to another

rotate-encryption-key reads:
  VIBEMDM_DATABASE_URL             database to update
  VIBEMDM_ENCRYPTION_KEY_OLD       current key ("" if secrets are stored unencrypted)
  VIBEMDM_ENCRYPTION_KEY           new key ("" to decrypt everything back to plaintext)
Stop the server first, run the command, then start the server with the new key.

Configuration (environment): VIBEMDM_DATABASE_URL (required), VIBEMDM_SERVER_URL,
VIBEMDM_LISTEN, VIBEMDM_DATA_DIR, VIBEMDM_AGENT_DIR, VIBEMDM_ADMIN_PASSWORD,
VIBEMDM_ENCRYPTION_KEY, VIBEMDM_TRUSTED_PROXIES, VIBEMDM_ALLOW_HTTP,
VIBEMDM_SESSION_TTL_HOURS, VIBEMDM_LOG_DB_LEVEL, VIBEMDM_DEBUG.
See man vibemdm (docs/man/vibemdm.8) and docs/manual/ for details.
`

func rotateKey() int {
	oldBox, err := secrets.New(os.Getenv("VIBEMDM_ENCRYPTION_KEY_OLD"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "old key:", err)
		return 2
	}
	newBox, err := secrets.New(os.Getenv("VIBEMDM_ENCRYPTION_KEY"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "new key:", err)
		return 2
	}
	dsn := os.Getenv("VIBEMDM_DATABASE_URL")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "VIBEMDM_DATABASE_URL is required")
		return 2
	}
	ctx := context.Background()
	sqlDB, err := db.Open(ctx, dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer sqlDB.Close()
	n, err := store.New(sqlDB).RekeySecrets(ctx, oldBox, newBox)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rotation failed, nothing was changed:", err)
		return 1
	}
	fmt.Printf("Re-encrypted %d stored secrets. Start the server with the new VIBEMDM_ENCRYPTION_KEY.\n", n)
	return 0
}

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version", "-version", "--version":
			fmt.Println(version)
			return
		case "rotate-encryption-key":
			os.Exit(rotateKey())
		case "help", "-h", "--help":
			fmt.Print(serverUsage)
			return
		}
	}
	cfg, err := config.FromEnv(version)
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		os.Exit(2)
	}
	level := slog.LevelInfo
	if cfg.Debug {
		level = slog.LevelDebug
	}
	var handler slog.Handler
	if os.Getenv("VIBEMDM_LOG_FORMAT") == "json" {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	} else {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	}
	logger := slog.New(handler)
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	srv, err := server.New(ctx, cfg, logger)
	if err != nil {
		logger.Error("startup failed", "err", err)
		os.Exit(1)
	}
	if err := srv.Run(ctx); err != nil {
		logger.Error("server error", "err", err)
		os.Exit(1)
	}
}
