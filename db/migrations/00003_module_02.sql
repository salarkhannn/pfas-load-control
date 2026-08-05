-- +goose Up
CREATE TABLE pfas.policy_rule_packs (
    id UUID PRIMARY KEY,
    code TEXT NOT NULL CHECK (length(btrim(code)) > 0),
    version TEXT NOT NULL CHECK (length(btrim(version)) > 0),
    jurisdiction TEXT NOT NULL CHECK (jurisdiction = 'MI'),
    authority_type TEXT NOT NULL CHECK (authority_type IN (
        'LAW', 'RULE', 'PERMIT_CONDITION', 'INTERIM_POLICY',
        'FINAL_GUIDANCE', 'DRAFT_GUIDANCE'
    )),
    effective_from DATE NOT NULL,
    effective_to DATE,
    retrieved_at TIMESTAMPTZ NOT NULL,
    source_url TEXT NOT NULL CHECK (source_url ~ '^https://'),
    source_title TEXT NOT NULL CHECK (length(btrim(source_title)) > 0),
    review_status TEXT NOT NULL CHECK (review_status IN ('DRAFT', 'ACTIVE', 'RETIRED')),
    reviewed_by TEXT,
    reviewed_at TIMESTAMPTZ,
    checksum TEXT NOT NULL CHECK (length(checksum) = 64),
    explanation TEXT NOT NULL,
    definition JSONB NOT NULL CHECK (jsonb_typeof(definition) = 'object'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (code, version),
    UNIQUE (id, checksum),
    CHECK (effective_to IS NULL OR effective_to >= effective_from),
    CHECK (
        (review_status = 'ACTIVE' AND reviewed_by IS NOT NULL AND reviewed_at IS NOT NULL AND authority_type <> 'DRAFT_GUIDANCE')
        OR review_status <> 'ACTIVE'
    )
);

CREATE UNIQUE INDEX policy_rule_packs_one_active_version
    ON pfas.policy_rule_packs(code, jurisdiction)
    WHERE review_status = 'ACTIVE';

CREATE TABLE pfas.batch_policy_decisions (
    id UUID PRIMARY KEY,
    workspace_id UUID NOT NULL REFERENCES pfas.workspaces(id) ON DELETE CASCADE,
    report_id UUID NOT NULL,
    report_version_id UUID NOT NULL,
    rule_pack_id UUID NOT NULL REFERENCES pfas.policy_rule_packs(id) ON DELETE RESTRICT,
    jurisdiction TEXT NOT NULL CHECK (jurisdiction = 'MI'),
    tier TEXT NOT NULL CHECK (tier IN ('STANDARD', 'ELEVATED', 'PROHIBITED', 'UNDETERMINED')),
    matched_rule_id TEXT,
    explanation TEXT NOT NULL,
    blocking_reason TEXT,
    input_hash TEXT NOT NULL CHECK (length(input_hash) = 64),
    analyte_evidence JSONB NOT NULL CHECK (jsonb_typeof(analyte_evidence) = 'array'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (report_id, workspace_id)
        REFERENCES pfas.lab_reports(id, workspace_id) ON DELETE RESTRICT,
    FOREIGN KEY (report_version_id, report_id)
        REFERENCES pfas.lab_report_versions(id, report_id) ON DELETE RESTRICT,
    UNIQUE (report_version_id, rule_pack_id),
    UNIQUE (id, workspace_id),
    CHECK ((tier = 'UNDETERMINED' AND blocking_reason IS NOT NULL AND matched_rule_id IS NULL)
        OR (tier <> 'UNDETERMINED' AND blocking_reason IS NULL AND matched_rule_id IS NOT NULL))
);

CREATE INDEX batch_policy_decisions_workspace_created_idx
    ON pfas.batch_policy_decisions(workspace_id, created_at DESC);

CREATE TABLE pfas.batch_policy_requirements (
    id UUID PRIMARY KEY,
    decision_id UUID NOT NULL REFERENCES pfas.batch_policy_decisions(id) ON DELETE CASCADE,
    position INTEGER NOT NULL CHECK (position > 0),
    requirement_id TEXT NOT NULL,
    title TEXT NOT NULL,
    detail TEXT NOT NULL,
    timing TEXT NOT NULL,
    rule_id TEXT NOT NULL,
    source_url TEXT NOT NULL CHECK (source_url ~ '^https://'),
    source_title TEXT NOT NULL,
    authority_type TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (decision_id, position),
    UNIQUE (decision_id, requirement_id)
);

-- +goose Down
DROP TABLE IF EXISTS pfas.batch_policy_requirements;
DROP TABLE IF EXISTS pfas.batch_policy_decisions;
DROP TABLE IF EXISTS pfas.policy_rule_packs;
