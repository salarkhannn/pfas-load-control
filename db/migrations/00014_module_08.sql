-- +goose Up
CREATE TABLE pfas.actions (
    id UUID PRIMARY KEY,
    workspace_id UUID NOT NULL REFERENCES pfas.workspaces(id) ON DELETE CASCADE,
    decision_package_id UUID NOT NULL,
    position INTEGER NOT NULL CHECK (position > 0),
    code TEXT NOT NULL CHECK (length(btrim(code)) > 0),
    category TEXT NOT NULL CHECK (length(btrim(category)) > 0),
    title TEXT NOT NULL CHECK (length(btrim(title)) > 0),
    detail TEXT NOT NULL CHECK (length(btrim(detail)) > 0),
    timing TEXT NOT NULL CHECK (length(btrim(timing)) > 0),
    source_id TEXT NOT NULL CHECK (length(btrim(source_id)) > 0),
    execution_mode TEXT NOT NULL CHECK (execution_mode IN ('INTERNAL_RELEASE', 'OPERATOR_HANDOFF', 'CONTROL')),
    status TEXT NOT NULL CHECK (status IN ('PROPOSED', 'APPROVED', 'REJECTED', 'EXECUTING', 'EXECUTED', 'FAILED')),
    approval_required BOOLEAN NOT NULL,
    channel TEXT NOT NULL CHECK (length(btrim(channel)) > 0),
    recipient TEXT NOT NULL,
    subject TEXT NOT NULL,
    message TEXT NOT NULL CHECK (length(btrim(message)) > 0),
    attachments JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(attachments) = 'array'),
    revision INTEGER NOT NULL DEFAULT 1 CHECK (revision > 0),
    payload_hash TEXT NOT NULL CHECK (length(payload_hash) = 64),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (decision_package_id, workspace_id)
        REFERENCES pfas.decision_packages(id, workspace_id) ON DELETE RESTRICT,
    UNIQUE (decision_package_id, position),
    UNIQUE (decision_package_id, code),
    UNIQUE (id, workspace_id),
    CHECK ((execution_mode = 'CONTROL' AND approval_required = false AND status = 'EXECUTED')
        OR execution_mode <> 'CONTROL')
);

CREATE INDEX actions_package_idx
    ON pfas.actions(decision_package_id, position);

CREATE TABLE pfas.action_decisions (
    id UUID PRIMARY KEY,
    workspace_id UUID NOT NULL REFERENCES pfas.workspaces(id) ON DELETE CASCADE,
    action_id UUID NOT NULL,
    decision_package_id UUID NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('APPROVED', 'REJECTED')),
    action_revision INTEGER NOT NULL CHECK (action_revision > 0),
    payload_hash TEXT NOT NULL CHECK (length(payload_hash) = 64),
    actor_name TEXT NOT NULL CHECK (length(btrim(actor_name)) BETWEEN 2 AND 120),
    actor_role TEXT NOT NULL CHECK (length(btrim(actor_role)) BETWEEN 2 AND 120),
    note TEXT NOT NULL DEFAULT '',
    acknowledged_gap_codes JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(acknowledged_gap_codes) = 'array'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (action_id, workspace_id)
        REFERENCES pfas.actions(id, workspace_id) ON DELETE RESTRICT,
    FOREIGN KEY (decision_package_id, workspace_id)
        REFERENCES pfas.decision_packages(id, workspace_id) ON DELETE RESTRICT,
    UNIQUE (action_id, action_revision),
    UNIQUE (id, workspace_id)
);

CREATE INDEX action_decisions_package_idx
    ON pfas.action_decisions(decision_package_id, created_at DESC);

CREATE TABLE pfas.execution_attempts (
    id UUID PRIMARY KEY,
    workspace_id UUID NOT NULL REFERENCES pfas.workspaces(id) ON DELETE CASCADE,
    action_id UUID NOT NULL,
    approval_id UUID NOT NULL,
    idempotency_key TEXT NOT NULL CHECK (length(btrim(idempotency_key)) BETWEEN 16 AND 128),
    outcome TEXT NOT NULL CHECK (outcome IN ('INTERNAL_RELEASED', 'OPERATOR_HANDOFF_READY')),
    receipt JSONB NOT NULL CHECK (jsonb_typeof(receipt) = 'object'),
    handoff_artifact BYTEA,
    handoff_sha256 TEXT CHECK (handoff_sha256 IS NULL OR length(handoff_sha256) = 64),
    completed_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (action_id, workspace_id)
        REFERENCES pfas.actions(id, workspace_id) ON DELETE RESTRICT,
    FOREIGN KEY (approval_id, workspace_id)
        REFERENCES pfas.action_decisions(id, workspace_id) ON DELETE RESTRICT,
    UNIQUE (action_id, idempotency_key),
    UNIQUE (id, workspace_id),
    CHECK ((outcome = 'OPERATOR_HANDOFF_READY' AND handoff_artifact IS NOT NULL AND handoff_sha256 IS NOT NULL)
        OR (outcome = 'INTERNAL_RELEASED' AND handoff_artifact IS NULL AND handoff_sha256 IS NULL))
);

CREATE INDEX execution_attempts_action_idx
    ON pfas.execution_attempts(action_id, created_at DESC);

CREATE TABLE pfas.placement_releases (
    id UUID PRIMARY KEY,
    workspace_id UUID NOT NULL REFERENCES pfas.workspaces(id) ON DELETE CASCADE,
    decision_package_id UUID NOT NULL,
    placement_evaluation_id UUID NOT NULL,
    action_id UUID NOT NULL,
    approval_id UUID NOT NULL,
    execution_attempt_id UUID NOT NULL,
    released_at TIMESTAMPTZ NOT NULL,
    FOREIGN KEY (decision_package_id, workspace_id)
        REFERENCES pfas.decision_packages(id, workspace_id) ON DELETE RESTRICT,
    FOREIGN KEY (placement_evaluation_id, workspace_id)
        REFERENCES pfas.placement_evaluations(id, workspace_id) ON DELETE RESTRICT,
    FOREIGN KEY (action_id, workspace_id)
        REFERENCES pfas.actions(id, workspace_id) ON DELETE RESTRICT,
    FOREIGN KEY (approval_id, workspace_id)
        REFERENCES pfas.action_decisions(id, workspace_id) ON DELETE RESTRICT,
    FOREIGN KEY (execution_attempt_id, workspace_id)
        REFERENCES pfas.execution_attempts(id, workspace_id) ON DELETE RESTRICT,
    UNIQUE (placement_evaluation_id),
    UNIQUE (execution_attempt_id)
);

-- +goose Down
DROP TABLE IF EXISTS pfas.placement_releases;
DROP TABLE IF EXISTS pfas.execution_attempts;
DROP TABLE IF EXISTS pfas.action_decisions;
DROP TABLE IF EXISTS pfas.actions;
