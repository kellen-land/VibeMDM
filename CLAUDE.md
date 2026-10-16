# VibeMDM

Self-hosted Apple MDM: Go server (embeds NanoMDM) + macOS agent + React console. Start with `HANDOFF.md` (local
working notes, git-ignored; may not exist in a fresh clone) and `TODO.md` for the current task list, then `README.md`,
`docs/architecture.md` and `docs/manual/`.

## Commands

- Build everything: `make web && make build && make sim && make agent` (UI is embedded: rebuild Go after `make web`).
- Gates before committing: `gofmt -l internal cmd` (empty), `go vet ./...`, `go test ./...`,
  `cd web && npm run typecheck && npm run check:ui && npm run build`.
- Local run needs PostgreSQL (`make dev-db`) and `VIBEMDM_*` env vars (see `docs/manual/02-installation.md`, and `HANDOFF.md` if present).
- Test without a Mac: `bin/vibemdm-sim` (`-no-mdm` for agent-only Macs).
- PostgreSQL integration tests (`*_integration_test.go`) skip unless `VIBEMDM_TEST_DSN` points at a throwaway
  database; CI does not set it, so run them locally for any `internal/store` or auth change. Agent code also needs
  `GOOS=darwin go vet ./internal/agent/... ./cmd/vibemdm-agent`.

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
- Security model lives in `docs/security.md`; update it (and its "Known limitations") with any change to auth,
  enrollment, the agent channel, secrets or the remote shell. Keep these invariants:
  - Every value sealed with `Box.Seal` has its column in `sealedColumns` (`internal/store/secrets.go`) so key
    rotation covers it; `TestSealedColumnsCoverSealCalls` fails otherwise.
  - Secret reveals (FileVault, LAPS, bypass codes) audit with `AuditStrict` and withhold the secret if that fails.
  - New viewer-readable fields that may hold secrets go through the redaction helpers; roles are only `admin`/`viewer`.
  - UDIDs and serial numbers are not proof of identity: never let them alone attach a device, agent or certificate
    to an existing record (see the MDM re-enrollment guard and agent takeover rules).
  - Anything the agent runs as root must come from a root-owned, non-writable path (`rootOwnedExecutable`).
  - Never record shell keystrokes or other input that may contain passwords.
  - Pin GitHub Actions to commit SHAs; Compose builds the local `vibemdm-local` image name.
- Migrations are applied by name, so parallel branches must not reuse a number.
- Keep Go and TypeScript API types in sync (`internal/model/model.go` ↔ `web/src/api/types.ts`).
- UI must stay calm and simple (owner is ADHD-sensitive to clutter): group features under nested menus/tabs like Jamf
  rather than adding top-level items, one primary action per screen, collapse advanced options.
  Follow `docs/ui-guidelines.md` (Index/Details/Settings templates, ObjectBar, one Save per object, sentence case).
