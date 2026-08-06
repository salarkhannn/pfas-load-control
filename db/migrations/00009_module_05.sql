-- +goose Up
CREATE TABLE pfas.placement_evaluations (
    id UUID PRIMARY KEY,
    workspace_id UUID NOT NULL REFERENCES pfas.workspaces(id) ON DELETE CASCADE,
    decision_id UUID NOT NULL,
    batch_id UUID NOT NULL,
    status TEXT NOT NULL CHECK (status IN (
        'READY', 'REVIEW_REQUIRED', 'INSUFFICIENT_CAPACITY', 'LAND_APPLICATION_BLOCKED'
    )),
    tier TEXT NOT NULL CHECK (tier IN ('STANDARD', 'ELEVATED', 'PROHIBITED')),
    config_version TEXT NOT NULL,
    config_checksum TEXT NOT NULL CHECK (length(config_checksum) = 64),
    input_hash TEXT NOT NULL CHECK (length(input_hash) = 64),
    wet_mass_kg NUMERIC,
    percent_solids NUMERIC,
    batch_dry_tons NUMERIC,
    allocated_dry_tons NUMERIC,
    unallocated_dry_tons NUMERIC,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (decision_id, workspace_id)
        REFERENCES pfas.batch_policy_decisions(id, workspace_id) ON DELETE RESTRICT,
    FOREIGN KEY (batch_id, workspace_id)
        REFERENCES pfas.biosolids_batches(id, workspace_id) ON DELETE RESTRICT,
    UNIQUE (decision_id, input_hash),
    UNIQUE (id, workspace_id),
    CHECK (wet_mass_kg IS NULL OR wet_mass_kg > 0),
    CHECK (percent_solids IS NULL OR (percent_solids > 0 AND percent_solids <= 100)),
    CHECK (batch_dry_tons IS NULL OR batch_dry_tons > 0),
    CHECK (allocated_dry_tons IS NULL OR allocated_dry_tons >= 0),
    CHECK (unallocated_dry_tons IS NULL OR unallocated_dry_tons >= 0),
    CHECK (
        batch_dry_tons IS NULL
        OR allocated_dry_tons IS NULL
        OR unallocated_dry_tons IS NULL
        OR batch_dry_tons = allocated_dry_tons + unallocated_dry_tons
    )
);

CREATE INDEX placement_evaluations_latest_idx
    ON pfas.placement_evaluations(decision_id, created_at DESC, id DESC);

CREATE TABLE pfas.placement_field_results (
    evaluation_id UUID NOT NULL REFERENCES pfas.placement_evaluations(id) ON DELETE CASCADE,
    field_id UUID NOT NULL REFERENCES pfas.candidate_fields(id) ON DELETE RESTRICT,
    physical_evaluation_id UUID REFERENCES pfas.physical_evaluations(id) ON DELETE RESTRICT,
    field_name TEXT NOT NULL,
    disposition TEXT NOT NULL CHECK (disposition IN ('ELIGIBLE', 'REVIEW_REQUIRED', 'INELIGIBLE')),
    rank INTEGER CHECK (rank IS NULL OR rank > 0),
    explanation TEXT NOT NULL,
    counterfactual TEXT,
    high_concern_count INTEGER NOT NULL CHECK (high_concern_count >= 0),
    moderate_concern_count INTEGER NOT NULL CHECK (moderate_concern_count >= 0),
    data_gap_count INTEGER NOT NULL CHECK (data_gap_count >= 0),
    allowed_rate_dry_tons_acre NUMERIC,
    available_capacity_dry_tons NUMERIC,
    road_access_distance_m DOUBLE PRECISION,
    reasons JSONB NOT NULL CHECK (jsonb_typeof(reasons) = 'array'),
    PRIMARY KEY (evaluation_id, field_id),
    CHECK (allowed_rate_dry_tons_acre IS NULL OR allowed_rate_dry_tons_acre > 0),
    CHECK (available_capacity_dry_tons IS NULL OR available_capacity_dry_tons >= 0),
    CHECK (road_access_distance_m IS NULL OR road_access_distance_m >= 0)
);

CREATE TABLE pfas.placement_vulnerability_categories (
    evaluation_id UUID NOT NULL,
    field_id UUID NOT NULL,
    category_key TEXT NOT NULL CHECK (category_key IN (
        'WATER_RECEPTORS', 'SUBSURFACE_MOBILITY', 'SURFACE_TRANSPORT',
        'HUMAN_FOOD_EXPOSURE', 'DATA_UNCERTAINTY'
    )),
    label TEXT NOT NULL,
    band TEXT NOT NULL CHECK (band IN ('LOW', 'MODERATE', 'HIGH', 'UNKNOWN')),
    explanation TEXT NOT NULL,
    components JSONB NOT NULL CHECK (jsonb_typeof(components) = 'array'),
    authority_type TEXT NOT NULL,
    source_title TEXT NOT NULL,
    source_url TEXT,
    config_version TEXT NOT NULL,
    PRIMARY KEY (evaluation_id, field_id, category_key),
    FOREIGN KEY (evaluation_id, field_id)
        REFERENCES pfas.placement_field_results(evaluation_id, field_id) ON DELETE CASCADE
);

CREATE TABLE pfas.placement_allocations (
    evaluation_id UUID NOT NULL REFERENCES pfas.placement_evaluations(id) ON DELETE CASCADE,
    position INTEGER NOT NULL CHECK (position > 0),
    field_id UUID NOT NULL,
    field_name TEXT NOT NULL,
    dry_tons NUMERIC NOT NULL CHECK (dry_tons > 0),
    acres NUMERIC NOT NULL CHECK (acres > 0),
    rate_dry_tons_acre NUMERIC NOT NULL CHECK (rate_dry_tons_acre > 0),
    PRIMARY KEY (evaluation_id, position),
    FOREIGN KEY (evaluation_id, field_id)
        REFERENCES pfas.placement_field_results(evaluation_id, field_id) ON DELETE RESTRICT,
    UNIQUE (evaluation_id, field_id)
);

CREATE TABLE pfas.placement_data_gaps (
    evaluation_id UUID NOT NULL REFERENCES pfas.placement_evaluations(id) ON DELETE CASCADE,
    code TEXT NOT NULL,
    detail TEXT NOT NULL,
    resolution TEXT NOT NULL,
    PRIMARY KEY (evaluation_id, code)
);

-- +goose Down
DROP TABLE IF EXISTS pfas.placement_data_gaps;
DROP TABLE IF EXISTS pfas.placement_allocations;
DROP TABLE IF EXISTS pfas.placement_vulnerability_categories;
DROP TABLE IF EXISTS pfas.placement_field_results;
DROP TABLE IF EXISTS pfas.placement_evaluations;
