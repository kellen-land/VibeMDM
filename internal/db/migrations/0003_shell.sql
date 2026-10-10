-- Remote shell session audit. The shell is disabled by default and gated by
-- the remote_shell_enabled server setting (Settings UI and PUT /api/v1/settings).
CREATE TABLE shell_sessions (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    computer_id  UUID NOT NULL REFERENCES computers (id) ON DELETE CASCADE,
    actor        TEXT NOT NULL DEFAULT '',
    actor_ip     TEXT NOT NULL DEFAULT '',
    status       TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','active','closed','denied','timeout','error')),
    reason       TEXT NOT NULL DEFAULT '',
    bytes_in     BIGINT NOT NULL DEFAULT 0,
    bytes_out    BIGINT NOT NULL DEFAULT 0,
    transcript   TEXT NOT NULL DEFAULT '',
    started_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    connected_at TIMESTAMPTZ,
    ended_at     TIMESTAMPTZ
);
CREATE INDEX shell_sessions_computer_idx ON shell_sessions (computer_id, started_at DESC);
CREATE INDEX shell_sessions_started_idx ON shell_sessions (started_at DESC);
