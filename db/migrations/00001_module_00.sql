-- +goose Up
CREATE SCHEMA IF NOT EXISTS pfas;
CREATE SCHEMA IF NOT EXISTS river;

CREATE TABLE pfas.agent_runs (
    id UUID PRIMARY KEY,
    kind TEXT NOT NULL CHECK (kind = 'MIREYE_READINESS'),
    status TEXT NOT NULL CHECK (status IN ('QUEUED', 'RUNNING', 'WAITING_FOR_INPUT', 'SUCCEEDED', 'FAILED', 'CANCELLED')),
    next_step SMALLINT NOT NULL CHECK (next_step BETWEEN 1 AND 4),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK ((status = 'SUCCEEDED' AND completed_at IS NOT NULL AND next_step = 4) OR status <> 'SUCCEEDED')
);

CREATE UNIQUE INDEX agent_runs_one_active_readiness
    ON pfas.agent_runs (kind)
    WHERE status IN ('QUEUED', 'RUNNING');

CREATE INDEX agent_runs_created_at_idx ON pfas.agent_runs (created_at DESC);

CREATE TABLE pfas.agent_steps (
    id UUID PRIMARY KEY,
    run_id UUID NOT NULL REFERENCES pfas.agent_runs(id) ON DELETE CASCADE,
    position SMALLINT NOT NULL CHECK (position BETWEEN 1 AND 3),
    tool_name TEXT NOT NULL CHECK (tool_name IN ('mireye.meta.fields', 'mireye.meta.plans', 'mireye.users.me.usage')),
    status TEXT NOT NULL CHECK (status IN ('PENDING', 'RUNNING', 'SUCCEEDED', 'FAILED')),
    attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    summary JSONB,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (run_id, position),
    UNIQUE (run_id, tool_name)
);

CREATE TABLE pfas.tool_calls (
    id UUID PRIMARY KEY,
    run_id UUID NOT NULL REFERENCES pfas.agent_runs(id) ON DELETE CASCADE,
    step_id UUID NOT NULL REFERENCES pfas.agent_steps(id) ON DELETE CASCADE,
    attempt INTEGER NOT NULL CHECK (attempt > 0),
    status TEXT NOT NULL CHECK (status IN ('SUCCEEDED', 'FAILED')),
    request_method TEXT NOT NULL CHECK (request_method = 'GET'),
    request_path TEXT NOT NULL,
    request_hash TEXT NOT NULL CHECK (length(request_hash) = 64),
    response_hash TEXT CHECK (response_hash IS NULL OR length(response_hash) = 64),
    request_id TEXT,
    source_url TEXT NOT NULL,
    http_status INTEGER CHECK (http_status IS NULL OR http_status BETWEEN 100 AND 599),
    duration_ms BIGINT NOT NULL CHECK (duration_ms >= 0),
    credit_cost INTEGER NOT NULL DEFAULT 0 CHECK (credit_cost = 0),
    response_body JSONB,
    error_code TEXT,
    fetched_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (step_id, attempt),
    CHECK (
        (status = 'SUCCEEDED' AND response_hash IS NOT NULL AND response_body IS NOT NULL AND error_code IS NULL)
        OR
        (status = 'FAILED' AND error_code IS NOT NULL)
    )
);

CREATE INDEX tool_calls_run_id_idx ON pfas.tool_calls (run_id, created_at);

CREATE TABLE pfas.data_gaps (
    id UUID PRIMARY KEY,
    run_id UUID NOT NULL REFERENCES pfas.agent_runs(id) ON DELETE CASCADE,
    step_id UUID REFERENCES pfas.agent_steps(id) ON DELETE CASCADE,
    code TEXT NOT NULL,
    detail TEXT NOT NULL,
    resolution TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('OPEN', 'RESOLVED')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at TIMESTAMPTZ,
    UNIQUE (run_id, step_id, code),
    CHECK ((status = 'RESOLVED' AND resolved_at IS NOT NULL) OR (status = 'OPEN' AND resolved_at IS NULL))
);

CREATE INDEX data_gaps_run_id_idx ON pfas.data_gaps (run_id, created_at);

-- +goose Down
DROP SCHEMA IF EXISTS pfas CASCADE;
DROP SCHEMA IF EXISTS river CASCADE;
