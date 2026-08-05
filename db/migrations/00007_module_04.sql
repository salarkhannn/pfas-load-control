-- +goose Up
CREATE TABLE pfas.physical_evaluations (
    id UUID PRIMARY KEY,
    workspace_id UUID NOT NULL REFERENCES pfas.workspaces(id) ON DELETE CASCADE,
    field_id UUID NOT NULL,
    geometry_id UUID NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('QUEUED', 'RUNNING', 'SUCCEEDED', 'REVIEW_REQUIRED', 'FAILED')),
    field_set_version TEXT NOT NULL,
    aggregation_version TEXT NOT NULL,
    catalog_version TEXT,
    catalog_etag TEXT,
    sample_count INTEGER NOT NULL DEFAULT 0 CHECK (sample_count BETWEEN 0 AND 25),
    projected_credits INTEGER NOT NULL DEFAULT 0 CHECK (projected_credits >= 0),
    request_hash TEXT CHECK (request_hash IS NULL OR length(request_hash) = 64),
    failure_code TEXT,
    failure_detail TEXT,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (field_id, workspace_id)
        REFERENCES pfas.candidate_fields(id, workspace_id) ON DELETE CASCADE,
    FOREIGN KEY (geometry_id, field_id)
        REFERENCES pfas.field_geometry_versions(id, field_id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX physical_evaluations_one_active_idx
    ON pfas.physical_evaluations(field_id)
    WHERE status IN ('QUEUED', 'RUNNING');

CREATE INDEX physical_evaluations_latest_idx
    ON pfas.physical_evaluations(field_id, created_at DESC, id DESC);

CREATE TABLE pfas.physical_sample_points (
    evaluation_id UUID NOT NULL REFERENCES pfas.physical_evaluations(id) ON DELETE CASCADE,
    sample_index INTEGER NOT NULL CHECK (sample_index BETWEEN 0 AND 24),
    label TEXT NOT NULL CHECK (length(btrim(label)) BETWEEN 1 AND 80),
    point extensions.geometry(Point, 4326) NOT NULL,
    PRIMARY KEY (evaluation_id, sample_index),
    CHECK (NOT extensions.ST_IsEmpty(point))
);

CREATE INDEX physical_sample_points_geometry_idx
    ON pfas.physical_sample_points USING GIST(point);

CREATE TABLE pfas.mireye_fetch_batches (
    id UUID PRIMARY KEY,
    evaluation_id UUID NOT NULL REFERENCES pfas.physical_evaluations(id) ON DELETE CASCADE,
    request_hash TEXT NOT NULL CHECK (length(request_hash) = 64),
    response_hash TEXT NOT NULL CHECK (length(response_hash) = 64),
    request_id TEXT,
    source_url TEXT NOT NULL,
    http_status INTEGER NOT NULL CHECK (http_status BETWEEN 100 AND 599),
    request JSONB NOT NULL CHECK (jsonb_typeof(request) = 'object'),
    response JSONB NOT NULL CHECK (jsonb_typeof(response) = 'object'),
    fetched_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (evaluation_id, request_hash)
);

CREATE TABLE pfas.physical_sample_facts (
    evaluation_id UUID NOT NULL REFERENCES pfas.physical_evaluations(id) ON DELETE CASCADE,
    sample_index INTEGER NOT NULL,
    field_name TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('ok', 'absent', 'failed')),
    value JSONB,
    unit TEXT,
    source TEXT,
    source_url TEXT,
    confidence TEXT,
    fetched_at TIMESTAMPTZ,
    dataset_vintage TEXT,
    ttl_seconds INTEGER CHECK (ttl_seconds IS NULL OR ttl_seconds >= 0),
    notes TEXT,
    error TEXT,
    retryable BOOLEAN NOT NULL DEFAULT false,
    PRIMARY KEY (evaluation_id, sample_index, field_name),
    FOREIGN KEY (evaluation_id, sample_index)
        REFERENCES pfas.physical_sample_points(evaluation_id, sample_index) ON DELETE CASCADE
);

CREATE TABLE pfas.physical_field_facts (
    evaluation_id UUID NOT NULL REFERENCES pfas.physical_evaluations(id) ON DELETE CASCADE,
    field_name TEXT NOT NULL,
    category TEXT NOT NULL CHECK (category IN ('WATER', 'SOIL', 'PEOPLE', 'LAND', 'ACCESS')),
    label TEXT NOT NULL,
    state TEXT NOT NULL CHECK (state IN ('COMPLETE', 'PARTIAL', 'UNAVAILABLE')),
    aggregate_method TEXT NOT NULL,
    value JSONB,
    unit TEXT,
    source TEXT,
    source_url TEXT,
    fetched_at TIMESTAMPTZ,
    ok_count INTEGER NOT NULL CHECK (ok_count >= 0),
    absent_count INTEGER NOT NULL CHECK (absent_count >= 0),
    failed_count INTEGER NOT NULL CHECK (failed_count >= 0),
    sample_indices JSONB NOT NULL CHECK (jsonb_typeof(sample_indices) = 'array'),
    critical BOOLEAN NOT NULL,
    PRIMARY KEY (evaluation_id, field_name)
);

CREATE TABLE pfas.supplemental_evidence (
    evaluation_id UUID NOT NULL REFERENCES pfas.physical_evaluations(id) ON DELETE CASCADE,
    provider TEXT NOT NULL CHECK (provider IN ('WELLOGIC', 'EPA_ECHO', 'OPERATOR_RECORD')),
    kind TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('AVAILABLE', 'UNAVAILABLE', 'NOT_PROVIDED')),
    title TEXT NOT NULL,
    summary TEXT NOT NULL,
    value JSONB,
    source_url TEXT NOT NULL,
    source_vintage TEXT,
    fetched_at TIMESTAMPTZ NOT NULL,
    caveat TEXT,
    PRIMARY KEY (evaluation_id, provider, kind)
);

CREATE TABLE pfas.physical_data_gaps (
    evaluation_id UUID NOT NULL REFERENCES pfas.physical_evaluations(id) ON DELETE CASCADE,
    code TEXT NOT NULL,
    source TEXT NOT NULL,
    detail TEXT NOT NULL,
    critical BOOLEAN NOT NULL,
    PRIMARY KEY (evaluation_id, code)
);

-- +goose Down
DROP TABLE IF EXISTS pfas.physical_data_gaps;
DROP TABLE IF EXISTS pfas.supplemental_evidence;
DROP TABLE IF EXISTS pfas.physical_field_facts;
DROP TABLE IF EXISTS pfas.physical_sample_facts;
DROP TABLE IF EXISTS pfas.mireye_fetch_batches;
DROP TABLE IF EXISTS pfas.physical_sample_points;
DROP TABLE IF EXISTS pfas.physical_evaluations;
