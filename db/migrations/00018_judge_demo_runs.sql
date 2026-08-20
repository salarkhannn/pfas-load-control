-- +goose Up

CREATE TABLE pfas.judge_demo_runs (
    id UUID PRIMARY KEY,
    idempotency_key TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL CHECK (status IN ('REVIEW_REQUIRED', 'SUCCEEDED', 'FAILED')),
    fixture_version TEXT NOT NULL,
    record JSONB NOT NULL,
    decision_hash CHAR(64) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX judge_demo_runs_created_at_idx ON pfas.judge_demo_runs (created_at DESC);

-- +goose Down

DROP TABLE IF EXISTS pfas.judge_demo_runs;
