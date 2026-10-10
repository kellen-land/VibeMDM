-- mSCP compliance baselines, scans and per-rule results.
CREATE TABLE compliance_baselines (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name                 TEXT NOT NULL UNIQUE,
    description          TEXT NOT NULL DEFAULT '',
    benchmark            TEXT NOT NULL,                       -- mSCP baseline key, e.g. cis_lvl1
    enabled              BOOLEAN NOT NULL DEFAULT TRUE,
    scope                JSONB NOT NULL DEFAULT '{}'::jsonb,
    extra_rules          JSONB NOT NULL DEFAULT '[]'::jsonb,  -- rule IDs added on top of the benchmark
    exclusions           JSONB NOT NULL DEFAULT '{}'::jsonb,  -- rule ID -> reason (exempt)
    odv_overrides        JSONB NOT NULL DEFAULT '{}'::jsonb,  -- rule ID -> value
    scan_interval_hours  INTEGER NOT NULL DEFAULT 24 CHECK (scan_interval_hours BETWEEN 1 AND 720),
    auto_remediate       BOOLEAN NOT NULL DEFAULT FALSE,      -- run mSCP fix scripts for failing rules
    deploy_profile       BOOLEAN NOT NULL DEFAULT FALSE,      -- enforce settings with a generated profile
    profile_id           UUID REFERENCES config_profiles (id) ON DELETE SET NULL,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE compliance_scans (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    baseline_id  UUID NOT NULL REFERENCES compliance_baselines (id) ON DELETE CASCADE,
    computer_id  UUID NOT NULL REFERENCES computers (id) ON DELETE CASCADE,
    task_id      UUID REFERENCES agent_tasks (id) ON DELETE SET NULL,
    trigger      TEXT NOT NULL DEFAULT 'scheduled',
    remediate    BOOLEAN NOT NULL DEFAULT FALSE,
    status       TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','completed','failed','expired')),
    os_version   TEXT NOT NULL DEFAULT '',
    passed       INTEGER NOT NULL DEFAULT 0,
    failed       INTEGER NOT NULL DEFAULT 0,
    remediated   INTEGER NOT NULL DEFAULT 0,
    errors       INTEGER NOT NULL DEFAULT 0,
    total        INTEGER NOT NULL DEFAULT 0,
    duration_ms  BIGINT NOT NULL DEFAULT 0,
    error        TEXT NOT NULL DEFAULT '',
    requested_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ
);
CREATE INDEX compliance_scans_lookup_idx ON compliance_scans (baseline_id, computer_id, requested_at DESC);

-- Latest result per (baseline, computer, rule); previous_status powers drift detection.
CREATE TABLE compliance_results (
    baseline_id     UUID NOT NULL REFERENCES compliance_baselines (id) ON DELETE CASCADE,
    computer_id     UUID NOT NULL REFERENCES computers (id) ON DELETE CASCADE,
    rule_id         TEXT NOT NULL,
    status          TEXT NOT NULL CHECK (status IN ('pass','fail','error','remediated')),
    previous_status TEXT NOT NULL DEFAULT '',
    actual          TEXT NOT NULL DEFAULT '',
    expected        TEXT NOT NULL DEFAULT '',
    detail          TEXT NOT NULL DEFAULT '',
    checked_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    changed_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (baseline_id, computer_id, rule_id)
);
CREATE INDEX compliance_results_rule_idx ON compliance_results (baseline_id, rule_id, status);
