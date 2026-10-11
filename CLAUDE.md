# VibeMDM

Self-hosted Apple MDM: Go server (embeds NanoMDM) + macOS agent + React console. Start with `HANDOFF.md` for the
current task list, then `README.md`, `docs/architecture.md` and `docs/manual/`.

## Commands

- Build everything: `make web && make build && make sim && make agent` (UI is embedded: rebuild Go after `make web`).
- Gates before committing: `gofmt -l internal cmd` (empty), `go vet ./...`, `go test ./...`,
  `cd web && npm run typecheck && npm run build`.
- Local run needs PostgreSQL (`make dev-db`) and `VIBEMDM_*` env vars (see `HANDOFF.md`, `docs/manual/02-installation.md`).
- Test without a Mac: `bin/vibemdm-sim` (`-no-mdm` for agent-only Macs).

## Layout

`cmd/{vibemdm,vibemdm-agent,vibemdm-sim,mscp-import}`, `internal/{api,agentapi,store,db,auth,mdmsvc,ddm,health,compliance,
onboarding,schema,superwf,mcp,secrets,agent,...}`, `web/src` (pages, components, api/queries.ts, api/types.ts), `deploy/`, `docs/`.
All SQL lives in `internal/store`. Admin routes are registered in `internal/api/api.go`; agent routes in `internal/agentapi`.

## Rules

- When you complete a TODO.md item, check it off and add a one-sentence summary sub-bullet in the same commit.
- Never commit secrets (`.env`, `*.pem`, `*.key` are ignored). Never log tokens, passwords, FileVault keys or SCEP challenges.
- Viewers may read configuration but not secrets: use the redaction helpers in `internal/api/redact.go`.
- Changes to how Macs are managed (profiles, shell commands, agent behaviour) need a note in the relevant doc, and
  anything that can only be proven on real hardware must be called out as untested.
- Stage explicit paths when committing (no `git commit -a`); keep commits small and messages specific.
- Keep Go and TypeScript API types in sync (`internal/model/model.go` ↔ `web/src/api/types.ts`).
- UI must stay calm and simple (owner is ADHD-sensitive to clutter): group features under nested menus/tabs like Jamf
  rather than adding top-level items, one primary action per screen, collapse advanced options.
