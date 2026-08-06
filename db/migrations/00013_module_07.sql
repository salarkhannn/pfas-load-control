-- +goose Up
CREATE TABLE pfas.decision_packages (
    id UUID PRIMARY KEY,
    workspace_id UUID NOT NULL REFERENCES pfas.workspaces(id) ON DELETE CASCADE,
    decision_id UUID NOT NULL,
    placement_evaluation_id UUID,
    response_run_id UUID,
    schema_version TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('READY', 'REVIEW_REQUIRED')),
    input_hash TEXT NOT NULL CHECK (length(input_hash) = 64),
    snapshot JSONB NOT NULL CHECK (jsonb_typeof(snapshot) = 'object'),
    evidence_ledger JSONB NOT NULL CHECK (jsonb_typeof(evidence_ledger) = 'array'),
    proposed_actions JSONB NOT NULL CHECK (jsonb_typeof(proposed_actions) = 'array'),
    json_artifact BYTEA NOT NULL,
    html_artifact TEXT NOT NULL,
    pdf_artifact BYTEA NOT NULL,
    json_sha256 TEXT NOT NULL CHECK (length(json_sha256) = 64),
    html_sha256 TEXT NOT NULL CHECK (length(html_sha256) = 64),
    pdf_sha256 TEXT NOT NULL CHECK (length(pdf_sha256) = 64),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (decision_id, workspace_id)
        REFERENCES pfas.batch_policy_decisions(id, workspace_id) ON DELETE RESTRICT,
    FOREIGN KEY (placement_evaluation_id, workspace_id)
        REFERENCES pfas.placement_evaluations(id, workspace_id) ON DELETE RESTRICT,
    FOREIGN KEY (response_run_id, workspace_id)
        REFERENCES pfas.response_runs(id, workspace_id) ON DELETE RESTRICT,
    UNIQUE (decision_id, input_hash),
    UNIQUE (id, workspace_id)
);

CREATE INDEX decision_packages_latest_idx
    ON pfas.decision_packages(decision_id, created_at DESC, id DESC);

-- +goose Down
DROP TABLE IF EXISTS pfas.decision_packages;
