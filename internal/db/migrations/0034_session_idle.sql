-- Console sessions end after a period without requests (VIBEMDM_SESSION_IDLE_MINUTES).
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now();
