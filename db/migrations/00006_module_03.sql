-- +goose Up
CREATE EXTENSION IF NOT EXISTS postgis WITH SCHEMA extensions;

CREATE TABLE pfas.candidate_fields (
    id UUID PRIMARY KEY,
    workspace_id UUID NOT NULL REFERENCES pfas.workspaces(id) ON DELETE CASCADE,
    facility_id UUID NOT NULL,
    name TEXT NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 160),
    normalized_name TEXT NOT NULL CHECK (length(normalized_name) BETWEEN 1 AND 160),
    locator_kind TEXT NOT NULL CHECK (locator_kind IN ('ADDRESS', 'COORDINATE', 'APN', 'GEOJSON')),
    locator_input TEXT NOT NULL CHECK (length(btrim(locator_input)) BETWEEN 1 AND 512),
    status TEXT NOT NULL CHECK (status IN ('NEEDS_LOCATION', 'NEEDS_GEOMETRY', 'NEEDS_DETAILS', 'READY')),
    mienviro_site_id TEXT CHECK (mienviro_site_id IS NULL OR length(btrim(mienviro_site_id)) BETWEEN 1 AND 120),
    rmp_approved BOOLEAN,
    rmp_document_reference TEXT CHECK (rmp_document_reference IS NULL OR length(btrim(rmp_document_reference)) <= 500),
    usable_acres NUMERIC CHECK (usable_acres IS NULL OR usable_acres > 0),
    crop_or_use TEXT CHECK (crop_or_use IS NULL OR length(btrim(crop_or_use)) <= 240),
    agronomic_rate_dry_tons_acre NUMERIC CHECK (agronomic_rate_dry_tons_acre IS NULL OR agronomic_rate_dry_tons_acre > 0),
    prior_loading_dry_tons NUMERIC CHECK (prior_loading_dry_tons IS NULL OR prior_loading_dry_tons >= 0),
    known_constraints TEXT CHECK (known_constraints IS NULL OR length(btrim(known_constraints)) <= 2000),
    access_constraints TEXT CHECK (access_constraints IS NULL OR length(btrim(access_constraints)) <= 2000),
    current_geometry_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (facility_id, workspace_id)
        REFERENCES pfas.facilities(id, workspace_id) ON DELETE CASCADE,
    UNIQUE (workspace_id, facility_id, normalized_name),
    UNIQUE (id, workspace_id)
);

CREATE INDEX candidate_fields_workspace_idx
    ON pfas.candidate_fields(workspace_id, facility_id, created_at DESC);

CREATE TABLE pfas.field_location_lookups (
    id UUID PRIMARY KEY,
    field_id UUID NOT NULL REFERENCES pfas.candidate_fields(id) ON DELETE CASCADE,
    workspace_id UUID NOT NULL REFERENCES pfas.workspaces(id) ON DELETE CASCADE,
    input TEXT NOT NULL CHECK (length(btrim(input)) BETWEEN 1 AND 512),
    input_kind TEXT NOT NULL CHECK (input_kind IN ('address', 'coord', 'apn')),
    request_hash TEXT NOT NULL CHECK (length(request_hash) = 64),
    response_hash TEXT NOT NULL CHECK (length(response_hash) = 64),
    request_id TEXT,
    source_url TEXT NOT NULL,
    disposition TEXT NOT NULL CHECK (disposition IN ('resolved', 'clarify', 'no_match')),
    latitude NUMERIC CHECK (latitude IS NULL OR (latitude >= -90 AND latitude <= 90)),
    longitude NUMERIC CHECK (longitude IS NULL OR (longitude >= -180 AND longitude <= 180)),
    resolved_address TEXT,
    state TEXT,
    county TEXT,
    fips TEXT,
    confidence NUMERIC CHECK (confidence IS NULL OR (confidence >= 0 AND confidence <= 1)),
    match_method TEXT,
    parcel_id TEXT,
    parcel_geometry JSONB,
    parcel_match_type TEXT,
    parcel_match_distance_m NUMERIC CHECK (parcel_match_distance_m IS NULL OR parcel_match_distance_m >= 0),
    parcel_source TEXT,
    parcel_unavailable BOOLEAN NOT NULL DEFAULT false,
    candidates JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(candidates) = 'array'),
    reason TEXT,
    hint TEXT,
    evidence JSONB NOT NULL CHECK (jsonb_typeof(evidence) = 'object'),
    fetched_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (field_id, workspace_id)
        REFERENCES pfas.candidate_fields(id, workspace_id) ON DELETE CASCADE,
    UNIQUE (field_id, request_hash),
    UNIQUE (id, field_id)
);

CREATE INDEX field_location_lookups_latest_idx
    ON pfas.field_location_lookups(field_id, fetched_at DESC, id DESC);

CREATE TABLE pfas.field_geometry_versions (
    id UUID PRIMARY KEY,
    field_id UUID NOT NULL REFERENCES pfas.candidate_fields(id) ON DELETE CASCADE,
    workspace_id UUID NOT NULL REFERENCES pfas.workspaces(id) ON DELETE CASCADE,
    version INTEGER NOT NULL CHECK (version > 0),
    source TEXT NOT NULL CHECK (source IN ('OPERATOR_GEOJSON', 'MIREYE_PARCEL_CONFIRMED')),
    source_lookup_id UUID,
    geometry extensions.geometry(MultiPolygon, 4326) NOT NULL,
    geometry_hash TEXT NOT NULL CHECK (length(geometry_hash) = 64),
    area_acres NUMERIC NOT NULL CHECK (area_acres > 0),
    confirmed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (field_id, workspace_id)
        REFERENCES pfas.candidate_fields(id, workspace_id) ON DELETE CASCADE,
    FOREIGN KEY (source_lookup_id, field_id)
        REFERENCES pfas.field_location_lookups(id, field_id) ON DELETE RESTRICT,
    UNIQUE (field_id, version),
    UNIQUE (field_id, geometry_hash),
    UNIQUE (id, field_id),
    CHECK (extensions.ST_IsValid(geometry)),
    CHECK (NOT extensions.ST_IsEmpty(geometry))
);

ALTER TABLE pfas.candidate_fields
    ADD CONSTRAINT candidate_fields_current_geometry_fk
    FOREIGN KEY (current_geometry_id, id)
    REFERENCES pfas.field_geometry_versions(id, field_id) ON DELETE RESTRICT;

CREATE TABLE pfas.field_data_gaps (
    id UUID PRIMARY KEY,
    field_id UUID NOT NULL REFERENCES pfas.candidate_fields(id) ON DELETE CASCADE,
    code TEXT NOT NULL CHECK (code IN (
        'LOCATION_UNRESOLVED',
        'LOCATION_AMBIGUOUS',
        'OUTSIDE_MICHIGAN',
        'GEOMETRY_UNCONFIRMED',
        'RMP_APPROVAL_MISSING',
        'USABLE_ACRES_MISSING',
        'AGRONOMIC_RATE_MISSING',
        'PRIOR_LOADING_MISSING'
    )),
    detail TEXT NOT NULL,
    resolution TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('OPEN', 'RESOLVED')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at TIMESTAMPTZ,
    UNIQUE (field_id, code),
    CHECK ((status = 'RESOLVED' AND resolved_at IS NOT NULL) OR (status = 'OPEN' AND resolved_at IS NULL))
);

CREATE INDEX field_data_gaps_open_idx
    ON pfas.field_data_gaps(field_id, created_at) WHERE status = 'OPEN';

-- +goose Down
DROP TABLE IF EXISTS pfas.field_data_gaps;
ALTER TABLE pfas.candidate_fields DROP CONSTRAINT IF EXISTS candidate_fields_current_geometry_fk;
DROP TABLE IF EXISTS pfas.field_geometry_versions;
DROP TABLE IF EXISTS pfas.field_location_lookups;
DROP TABLE IF EXISTS pfas.candidate_fields;
