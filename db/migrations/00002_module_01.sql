-- +goose Up
CREATE TABLE pfas.workspaces (
    id UUID PRIMARY KEY,
    key_hash TEXT NOT NULL UNIQUE CHECK (length(key_hash) = 64),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE pfas.facilities (
    id UUID PRIMARY KEY,
    workspace_id UUID NOT NULL REFERENCES pfas.workspaces(id) ON DELETE CASCADE,
    name TEXT NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 160),
    normalized_name TEXT NOT NULL CHECK (length(normalized_name) BETWEEN 1 AND 160),
    jurisdiction TEXT NOT NULL DEFAULT 'MI' CHECK (jurisdiction = 'MI'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, normalized_name),
    UNIQUE (id, workspace_id)
);

CREATE TABLE pfas.biosolids_batches (
    id UUID PRIMARY KEY,
    workspace_id UUID NOT NULL REFERENCES pfas.workspaces(id) ON DELETE CASCADE,
    facility_id UUID NOT NULL,
    identifier TEXT NOT NULL CHECK (length(btrim(identifier)) BETWEEN 1 AND 120),
    normalized_identifier TEXT NOT NULL CHECK (length(normalized_identifier) BETWEEN 1 AND 120),
    wet_mass_kg NUMERIC,
    percent_solids NUMERIC,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (facility_id, workspace_id)
        REFERENCES pfas.facilities(id, workspace_id) ON DELETE CASCADE,
    UNIQUE (facility_id, normalized_identifier),
    UNIQUE (id, workspace_id),
    CHECK (wet_mass_kg IS NULL OR wet_mass_kg > 0),
    CHECK (percent_solids IS NULL OR (percent_solids > 0 AND percent_solids <= 100))
);

CREATE TABLE pfas.lab_reports (
    id UUID PRIMARY KEY,
    workspace_id UUID NOT NULL REFERENCES pfas.workspaces(id) ON DELETE CASCADE,
    facility_id UUID NOT NULL,
    batch_id UUID NOT NULL,
    original_filename TEXT NOT NULL CHECK (length(btrim(original_filename)) BETWEEN 1 AND 255),
    media_type TEXT NOT NULL CHECK (media_type IN ('application/pdf', 'text/csv', 'application/json')),
    size_bytes INTEGER NOT NULL CHECK (size_bytes BETWEEN 1 AND 10485760),
    sha256 TEXT NOT NULL CHECK (length(sha256) = 64),
    content BYTEA NOT NULL,
    status TEXT NOT NULL CHECK (status IN (
        'UPLOADED', 'PROCESSING', 'NEEDS_REVIEW', 'READY_TO_CONFIRM', 'CONFIRMED', 'FAILED'
    )),
    current_version INTEGER NOT NULL DEFAULT 0 CHECK (current_version >= 0),
    failure_code TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    confirmed_at TIMESTAMPTZ,
    FOREIGN KEY (facility_id, workspace_id)
        REFERENCES pfas.facilities(id, workspace_id) ON DELETE RESTRICT,
    FOREIGN KEY (batch_id, workspace_id)
        REFERENCES pfas.biosolids_batches(id, workspace_id) ON DELETE RESTRICT,
    UNIQUE (workspace_id, sha256),
    UNIQUE (id, workspace_id),
    CHECK (octet_length(content) = size_bytes),
    CHECK ((status = 'FAILED' AND failure_code IS NOT NULL) OR (status <> 'FAILED' AND failure_code IS NULL)),
    CHECK ((status = 'CONFIRMED' AND confirmed_at IS NOT NULL) OR (status <> 'CONFIRMED' AND confirmed_at IS NULL))
);

CREATE INDEX lab_reports_workspace_created_idx
    ON pfas.lab_reports(workspace_id, created_at DESC);

CREATE TABLE pfas.lab_report_pages (
    report_id UUID NOT NULL REFERENCES pfas.lab_reports(id) ON DELETE CASCADE,
    page_number INTEGER NOT NULL CHECK (page_number > 0),
    extracted_text TEXT NOT NULL,
    extraction_method TEXT NOT NULL CHECK (extraction_method IN ('PDF_TEXT', 'OCR', 'CSV', 'JSON')),
    width NUMERIC,
    height NUMERIC,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (report_id, page_number),
    CHECK ((width IS NULL AND height IS NULL) OR (width > 0 AND height > 0))
);

CREATE TABLE pfas.lab_report_versions (
    id UUID PRIMARY KEY,
    report_id UUID NOT NULL REFERENCES pfas.lab_reports(id) ON DELETE CASCADE,
    version INTEGER NOT NULL CHECK (version > 0),
    status TEXT NOT NULL CHECK (status IN ('DRAFT', 'CONFIRMED', 'SUPERSEDED')),
    source TEXT NOT NULL CHECK (source IN ('EXTRACTION', 'OPERATOR_CORRECTION')),
    laboratory TEXT,
    sample_identifier TEXT,
    collection_date DATE,
    matrix TEXT,
    method TEXT,
    basis TEXT CHECK (basis IS NULL OR basis IN ('DRY', 'WET')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    confirmed_at TIMESTAMPTZ,
    UNIQUE (report_id, version),
    UNIQUE (id, report_id),
    CHECK ((status = 'CONFIRMED' AND confirmed_at IS NOT NULL) OR (status <> 'CONFIRMED' AND confirmed_at IS NULL))
);

CREATE UNIQUE INDEX lab_report_versions_one_confirmed
    ON pfas.lab_report_versions(report_id)
    WHERE status = 'CONFIRMED';

CREATE TABLE pfas.analyte_results (
    id UUID PRIMARY KEY,
    report_id UUID NOT NULL REFERENCES pfas.lab_reports(id) ON DELETE CASCADE,
    version_id UUID NOT NULL,
    canonical_analyte TEXT NOT NULL CHECK (canonical_analyte IN ('PFOS', 'PFOA')),
    reported_analyte TEXT NOT NULL CHECK (length(btrim(reported_analyte)) > 0),
    result_text TEXT NOT NULL CHECK (length(btrim(result_text)) > 0),
    value NUMERIC,
    unit TEXT CHECK (unit IS NULL OR unit IN ('UG_KG', 'NG_G', 'MG_KG', 'UG_L')),
    basis TEXT CHECK (basis IS NULL OR basis IN ('DRY', 'WET')),
    qualifier TEXT,
    is_non_detect BOOLEAN NOT NULL DEFAULT false,
    reporting_limit NUMERIC,
    detection_limit NUMERIC,
    normalized_value_ug_kg_dry NUMERIC,
    normalized_reporting_limit_ug_kg_dry NUMERIC,
    normalized_detection_limit_ug_kg_dry NUMERIC,
    source_page INTEGER NOT NULL CHECK (source_page > 0),
    source_excerpt TEXT NOT NULL CHECK (length(btrim(source_excerpt)) > 0),
    source_bounds JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (version_id, report_id)
        REFERENCES pfas.lab_report_versions(id, report_id) ON DELETE CASCADE,
    UNIQUE (version_id, canonical_analyte),
    CHECK (value IS NOT NULL OR is_non_detect),
    CHECK (NOT is_non_detect OR value IS NULL),
    CHECK (reporting_limit IS NULL OR reporting_limit >= 0),
    CHECK (detection_limit IS NULL OR detection_limit >= 0),
    CHECK (normalized_value_ug_kg_dry IS NULL OR normalized_value_ug_kg_dry >= 0),
    CHECK (normalized_reporting_limit_ug_kg_dry IS NULL OR normalized_reporting_limit_ug_kg_dry >= 0),
    CHECK (normalized_detection_limit_ug_kg_dry IS NULL OR normalized_detection_limit_ug_kg_dry >= 0)
);

CREATE TABLE pfas.lab_report_gaps (
    id UUID PRIMARY KEY,
    report_id UUID NOT NULL REFERENCES pfas.lab_reports(id) ON DELETE CASCADE,
    version_id UUID NOT NULL,
    code TEXT NOT NULL,
    field_name TEXT NOT NULL,
    detail TEXT NOT NULL,
    resolution TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('OPEN', 'RESOLVED')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at TIMESTAMPTZ,
    FOREIGN KEY (version_id, report_id)
        REFERENCES pfas.lab_report_versions(id, report_id) ON DELETE CASCADE,
    UNIQUE (version_id, code, field_name),
    CHECK ((status = 'RESOLVED' AND resolved_at IS NOT NULL) OR (status = 'OPEN' AND resolved_at IS NULL))
);

CREATE INDEX lab_report_gaps_report_idx
    ON pfas.lab_report_gaps(report_id, status, created_at);

CREATE TABLE pfas.lab_confirmations (
    id UUID PRIMARY KEY,
    workspace_id UUID NOT NULL REFERENCES pfas.workspaces(id) ON DELETE CASCADE,
    report_id UUID NOT NULL,
    version_id UUID NOT NULL,
    evidence_hash TEXT NOT NULL CHECK (length(evidence_hash) = 64),
    confirmed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (report_id, workspace_id)
        REFERENCES pfas.lab_reports(id, workspace_id) ON DELETE CASCADE,
    FOREIGN KEY (version_id, report_id)
        REFERENCES pfas.lab_report_versions(id, report_id) ON DELETE RESTRICT,
    UNIQUE (version_id)
);

-- +goose Down
DROP TABLE IF EXISTS pfas.lab_confirmations;
DROP TABLE IF EXISTS pfas.lab_report_gaps;
DROP TABLE IF EXISTS pfas.analyte_results;
DROP TABLE IF EXISTS pfas.lab_report_versions;
DROP TABLE IF EXISTS pfas.lab_report_pages;
DROP TABLE IF EXISTS pfas.lab_reports;
DROP TABLE IF EXISTS pfas.biosolids_batches;
DROP TABLE IF EXISTS pfas.facilities;
DROP TABLE IF EXISTS pfas.workspaces;
