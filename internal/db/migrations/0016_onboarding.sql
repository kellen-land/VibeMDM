-- White-glove onboarding workflows (swiftDialog) and their runs.
CREATE TABLE onboarding_workflows (
    id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name                   TEXT NOT NULL UNIQUE,
    enabled                BOOLEAN NOT NULL DEFAULT TRUE,
    priority               INTEGER NOT NULL DEFAULT 100,   -- lower wins when several are in scope
    scope                  JSONB NOT NULL DEFAULT '{}'::jsonb,
    config                 JSONB NOT NULL DEFAULT '{}'::jsonb, -- branding, fields, steps, final action
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE onboarding_runs (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workflow_id  UUID NOT NULL REFERENCES onboarding_workflows (id) ON DELETE CASCADE,
    computer_id  UUID NOT NULL REFERENCES computers (id) ON DELETE CASCADE,
    task_id      UUID REFERENCES agent_tasks (id) ON DELETE SET NULL,
    status       TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','running','completed','failed')),
    user_entry   JSONB NOT NULL DEFAULT '{}'::jsonb,
    steps        JSONB NOT NULL DEFAULT '[]'::jsonb,
    log          TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at   TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    UNIQUE (workflow_id, computer_id)
);
CREATE INDEX onboarding_runs_computer_idx ON onboarding_runs (computer_id);
