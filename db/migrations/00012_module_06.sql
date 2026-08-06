-- +goose Up
CREATE TABLE pfas.facility_location_lookups (
    id UUID PRIMARY KEY,
    workspace_id UUID NOT NULL REFERENCES pfas.workspaces(id) ON DELETE CASCADE,
    facility_id UUID NOT NULL,
    input TEXT NOT NULL CHECK (length(btrim(input)) BETWEEN 1 AND 256),
    input_kind TEXT NOT NULL CHECK (input_kind IN ('address', 'coord')),
    request_hash TEXT NOT NULL CHECK (length(request_hash) = 64),
    response_hash TEXT NOT NULL CHECK (length(response_hash) = 64),
    request_id TEXT,
    source_url TEXT NOT NULL CHECK (source_url ~ '^https://'),
    disposition TEXT NOT NULL CHECK (disposition IN ('resolved', 'clarify', 'no_match')),
    latitude DOUBLE PRECISION,
    longitude DOUBLE PRECISION,
    resolved_address TEXT,
    state TEXT,
    county TEXT,
    confidence DOUBLE PRECISION,
    match_method TEXT,
    candidates JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(candidates) = 'array'),
    reason TEXT,
    hint TEXT,
    evidence JSONB NOT NULL CHECK (jsonb_typeof(evidence) = 'object'),
    fetched_at TIMESTAMPTZ NOT NULL,
    confirmed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (facility_id, workspace_id)
        REFERENCES pfas.facilities(id, workspace_id) ON DELETE CASCADE,
    UNIQUE (facility_id, request_hash),
    UNIQUE (id, workspace_id),
    CHECK (confidence IS NULL OR (confidence >= 0 AND confidence <= 1)),
    CHECK ((latitude IS NULL AND longitude IS NULL) OR
           (latitude BETWEEN -90 AND 90 AND longitude BETWEEN -180 AND 180)),
    CHECK (confirmed_at IS NULL OR (disposition = 'resolved' AND latitude IS NOT NULL AND longitude IS NOT NULL AND state = 'MI'))
);

CREATE TABLE pfas.response_runs (
    id UUID PRIMARY KEY,
    workspace_id UUID NOT NULL REFERENCES pfas.workspaces(id) ON DELETE CASCADE,
    decision_id UUID NOT NULL,
    facility_location_id UUID NOT NULL,
    tier TEXT NOT NULL CHECK (tier IN ('ELEVATED', 'PROHIBITED')),
    status TEXT NOT NULL CHECK (status IN ('QUEUED', 'RUNNING', 'READY', 'REVIEW_REQUIRED', 'FAILED')),
    input_hash TEXT NOT NULL CHECK (length(input_hash) = 64),
    policy_source_url TEXT NOT NULL CHECK (policy_source_url ~ '^https://'),
    policy_version TEXT NOT NULL,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    failure_code TEXT,
    failure_detail TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (decision_id, workspace_id)
        REFERENCES pfas.batch_policy_decisions(id, workspace_id) ON DELETE RESTRICT,
    FOREIGN KEY (facility_location_id, workspace_id)
        REFERENCES pfas.facility_location_lookups(id, workspace_id) ON DELETE RESTRICT,
    UNIQUE (decision_id, input_hash),
    UNIQUE (id, workspace_id),
    CHECK ((status = 'FAILED' AND failure_code IS NOT NULL AND failure_detail IS NOT NULL)
        OR (status <> 'FAILED' AND failure_code IS NULL AND failure_detail IS NULL)),
    CHECK ((status IN ('READY', 'REVIEW_REQUIRED', 'FAILED') AND completed_at IS NOT NULL)
        OR status IN ('QUEUED', 'RUNNING'))
);

CREATE UNIQUE INDEX response_runs_one_active
    ON pfas.response_runs(decision_id)
    WHERE status IN ('QUEUED', 'RUNNING');

CREATE INDEX response_runs_latest_idx
    ON pfas.response_runs(decision_id, created_at DESC, id DESC);

CREATE TABLE pfas.response_tasks (
    run_id UUID NOT NULL REFERENCES pfas.response_runs(id) ON DELETE CASCADE,
    position INTEGER NOT NULL CHECK (position > 0),
    code TEXT NOT NULL,
    category TEXT NOT NULL CHECK (category IN ('CONTROL', 'REGULATORY', 'SAMPLING', 'INVESTIGATION', 'ALTERNATIVE_MANAGEMENT')),
    title TEXT NOT NULL,
    detail TEXT NOT NULL,
    timing TEXT NOT NULL,
    state TEXT NOT NULL CHECK (state IN ('ENFORCED', 'REQUIRED', 'DRAFT')),
    PRIMARY KEY (run_id, position),
    UNIQUE (run_id, code)
);

CREATE TABLE pfas.response_evidence (
    id UUID PRIMARY KEY,
    run_id UUID NOT NULL REFERENCES pfas.response_runs(id) ON DELETE CASCADE,
    provider TEXT NOT NULL,
    kind TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('AVAILABLE', 'PARTIAL', 'UNAVAILABLE')),
    title TEXT NOT NULL,
    summary TEXT NOT NULL,
    data JSONB NOT NULL CHECK (jsonb_typeof(data) IN ('object', 'array')),
    source_url TEXT NOT NULL CHECK (source_url ~ '^https://'),
    source_vintage TEXT,
    request_hash TEXT,
    response_hash TEXT,
    request_id TEXT,
    fetched_at TIMESTAMPTZ NOT NULL,
    caveat TEXT NOT NULL,
    UNIQUE (run_id, provider, kind)
);

CREATE TABLE pfas.investigation_leads (
    run_id UUID NOT NULL REFERENCES pfas.response_runs(id) ON DELETE CASCADE,
    position INTEGER NOT NULL CHECK (position > 0),
    registry_id TEXT NOT NULL,
    facility_name TEXT NOT NULL,
    city TEXT,
    state TEXT,
    naics_codes JSONB NOT NULL CHECK (jsonb_typeof(naics_codes) = 'array'),
    evidence_tier INTEGER NOT NULL CHECK (evidence_tier BETWEEN 1 AND 4),
    evidence_label TEXT NOT NULL,
    rationale TEXT NOT NULL,
    caveat TEXT NOT NULL,
    source_url TEXT NOT NULL CHECK (source_url ~ '^https://'),
    PRIMARY KEY (run_id, position),
    UNIQUE (run_id, registry_id)
);

CREATE TABLE pfas.alternative_management_candidates (
    run_id UUID NOT NULL REFERENCES pfas.response_runs(id) ON DELETE CASCADE,
    position INTEGER NOT NULL CHECK (position > 0),
    wds_id TEXT NOT NULL,
    facility_name TEXT NOT NULL,
    facility_type TEXT NOT NULL,
    address TEXT NOT NULL,
    city TEXT NOT NULL,
    county TEXT NOT NULL,
    latitude DOUBLE PRECISION NOT NULL CHECK (latitude BETWEEN -90 AND 90),
    longitude DOUBLE PRECISION NOT NULL CHECK (longitude BETWEEN -180 AND 180),
    disposal_area_status TEXT NOT NULL,
    straightline_distance_km DOUBLE PRECISION NOT NULL CHECK (straightline_distance_km >= 0),
    route_status TEXT NOT NULL CHECK (route_status IN ('ROUTED', 'UNREACHABLE', 'DROPPED', 'NOT_ROUTED')),
    driving_distance_km DOUBLE PRECISION CHECK (driving_distance_km IS NULL OR driving_distance_km >= 0),
    duration_minutes DOUBLE PRECISION CHECK (duration_minutes IS NULL OR duration_minutes >= 0),
    route_note TEXT,
    acceptance_status TEXT NOT NULL DEFAULT 'UNVERIFIED' CHECK (acceptance_status = 'UNVERIFIED'),
    executable BOOLEAN NOT NULL DEFAULT false CHECK (executable = false),
    source_url TEXT NOT NULL CHECK (source_url ~ '^https://'),
    PRIMARY KEY (run_id, position),
    UNIQUE (run_id, wds_id)
);

CREATE TABLE pfas.response_data_gaps (
    run_id UUID NOT NULL REFERENCES pfas.response_runs(id) ON DELETE CASCADE,
    code TEXT NOT NULL,
    detail TEXT NOT NULL,
    resolution TEXT NOT NULL,
    critical BOOLEAN NOT NULL,
    PRIMARY KEY (run_id, code)
);

-- +goose Down
DROP TABLE IF EXISTS pfas.response_data_gaps;
DROP TABLE IF EXISTS pfas.alternative_management_candidates;
DROP TABLE IF EXISTS pfas.investigation_leads;
DROP TABLE IF EXISTS pfas.response_evidence;
DROP TABLE IF EXISTS pfas.response_tasks;
DROP TABLE IF EXISTS pfas.response_runs;
DROP TABLE IF EXISTS pfas.facility_location_lookups;
