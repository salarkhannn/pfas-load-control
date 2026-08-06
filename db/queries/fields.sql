-- name: GetFacilityForWorkspace :one
SELECT id, name, jurisdiction
FROM pfas.facilities
WHERE id = $1 AND workspace_id = $2;

-- name: CreateCandidateField :one
INSERT INTO pfas.candidate_fields (
    id, workspace_id, facility_id, name, normalized_name,
    locator_kind, locator_input, status
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetCandidateFieldBase :one
SELECT *
FROM pfas.candidate_fields
WHERE id = $1 AND workspace_id = $2;

-- name: ListCandidateFieldRows :many
SELECT f.id, f.facility_id, facility.name AS facility_name, f.name, f.locator_kind,
       f.locator_input, f.status, f.mienviro_site_id, f.rmp_approved,
       f.rmp_document_reference,
       CAST(COALESCE(f.usable_acres::text, '') AS text) AS usable_acres,
       f.crop_or_use,
       CAST(COALESCE(f.agronomic_rate_dry_tons_acre::text, '') AS text) AS agronomic_rate_dry_tons_acre,
       CAST(COALESCE(f.prior_loading_dry_tons::text, '') AS text) AS prior_loading_dry_tons,
       f.known_constraints, f.access_constraints, f.current_geometry_id,
       f.created_at, f.updated_at,
       (lookup.id IS NOT NULL) AS has_lookup,
       CAST(COALESCE(lookup.id::text, '') AS text) AS lookup_id,
       CAST(COALESCE(lookup.disposition, '') AS text) AS lookup_disposition,
       CAST(COALESCE(lookup.latitude::text, '') AS text) AS latitude,
       CAST(COALESCE(lookup.longitude::text, '') AS text) AS longitude,
       lookup.resolved_address, lookup.state, lookup.county, lookup.fips,
       CAST(COALESCE(lookup.confidence::text, '') AS text) AS confidence,
       lookup.match_method, lookup.parcel_id, lookup.parcel_geometry,
       lookup.parcel_match_type,
       CAST(COALESCE(lookup.parcel_match_distance_m::text, '') AS text) AS parcel_match_distance_m,
       lookup.parcel_source, COALESCE(lookup.parcel_unavailable, false) AS parcel_unavailable,
       COALESCE(lookup.candidates, '[]'::jsonb) AS candidates,
       lookup.reason, lookup.hint, lookup.request_id,
       CAST(COALESCE(lookup.source_url, '') AS text) AS source_url,
       CAST(COALESCE(lookup.response_hash, '') AS text) AS response_hash,
       lookup.fetched_at,
       geometry.version AS geometry_version, geometry.source AS geometry_source,
       CAST(COALESCE(extensions.ST_AsGeoJSON(geometry.geometry), '') AS text) AS geometry_geojson,
       CAST(COALESCE(geometry.area_acres::text, '') AS text) AS geometry_area_acres,
       geometry.geometry_hash, geometry.confirmed_at AS geometry_confirmed_at
FROM pfas.candidate_fields f
JOIN pfas.facilities facility
  ON facility.id = f.facility_id AND facility.workspace_id = f.workspace_id
LEFT JOIN LATERAL (
    SELECT l.* FROM pfas.field_location_lookups l
    WHERE l.field_id = f.id
    ORDER BY l.fetched_at DESC, l.id DESC
    LIMIT 1
) lookup ON true
LEFT JOIN pfas.field_geometry_versions geometry
  ON geometry.id = f.current_geometry_id AND geometry.field_id = f.id
WHERE f.workspace_id = $1
  AND (sqlc.narg(facility_id)::uuid IS NULL OR f.facility_id = sqlc.narg(facility_id))
  AND (sqlc.narg(field_id)::uuid IS NULL OR f.id = sqlc.narg(field_id))
ORDER BY f.created_at DESC, f.id;

-- name: GetCandidateFieldRow :one
SELECT f.id, f.facility_id, facility.name AS facility_name, f.name, f.locator_kind,
       f.locator_input, f.status, f.mienviro_site_id, f.rmp_approved,
       f.rmp_document_reference,
       CAST(COALESCE(f.usable_acres::text, '') AS text) AS usable_acres,
       f.crop_or_use,
       CAST(COALESCE(f.agronomic_rate_dry_tons_acre::text, '') AS text) AS agronomic_rate_dry_tons_acre,
       CAST(COALESCE(f.prior_loading_dry_tons::text, '') AS text) AS prior_loading_dry_tons,
       f.known_constraints, f.access_constraints, f.current_geometry_id,
       f.created_at, f.updated_at,
       (lookup.id IS NOT NULL) AS has_lookup,
       CAST(COALESCE(lookup.id::text, '') AS text) AS lookup_id,
       CAST(COALESCE(lookup.disposition, '') AS text) AS lookup_disposition,
       CAST(COALESCE(lookup.latitude::text, '') AS text) AS latitude,
       CAST(COALESCE(lookup.longitude::text, '') AS text) AS longitude,
       lookup.resolved_address, lookup.state, lookup.county, lookup.fips,
       CAST(COALESCE(lookup.confidence::text, '') AS text) AS confidence,
       lookup.match_method, lookup.parcel_id, lookup.parcel_geometry,
       lookup.parcel_match_type,
       CAST(COALESCE(lookup.parcel_match_distance_m::text, '') AS text) AS parcel_match_distance_m,
       lookup.parcel_source, COALESCE(lookup.parcel_unavailable, false) AS parcel_unavailable,
       COALESCE(lookup.candidates, '[]'::jsonb) AS candidates,
       lookup.reason, lookup.hint, lookup.request_id,
       CAST(COALESCE(lookup.source_url, '') AS text) AS source_url,
       CAST(COALESCE(lookup.response_hash, '') AS text) AS response_hash,
       lookup.fetched_at,
       geometry.version AS geometry_version, geometry.source AS geometry_source,
       CAST(COALESCE(extensions.ST_AsGeoJSON(geometry.geometry), '') AS text) AS geometry_geojson,
       CAST(COALESCE(geometry.area_acres::text, '') AS text) AS geometry_area_acres,
       geometry.geometry_hash, geometry.confirmed_at AS geometry_confirmed_at
FROM pfas.candidate_fields f
JOIN pfas.facilities facility
  ON facility.id = f.facility_id AND facility.workspace_id = f.workspace_id
LEFT JOIN LATERAL (
    SELECT l.* FROM pfas.field_location_lookups l
    WHERE l.field_id = f.id
    ORDER BY l.fetched_at DESC, l.id DESC
    LIMIT 1
) lookup ON true
LEFT JOIN pfas.field_geometry_versions geometry
  ON geometry.id = f.current_geometry_id AND geometry.field_id = f.id
WHERE f.id = $1 AND f.workspace_id = $2;

-- name: GetLocationLookupByRequestHash :one
SELECT * FROM pfas.field_location_lookups
WHERE field_id = $1 AND request_hash = $2;

-- name: GetLatestLocationLookup :one
SELECT * FROM pfas.field_location_lookups
WHERE field_id = $1
ORDER BY fetched_at DESC, id DESC
LIMIT 1;

-- name: GetCandidateFieldForUpdate :one
SELECT *
FROM pfas.candidate_fields
WHERE id = $1 AND workspace_id = $2
FOR UPDATE;

-- name: CreateLocationLookup :one
INSERT INTO pfas.field_location_lookups (
    id, field_id, workspace_id, input, input_kind, request_hash, response_hash,
    request_id, source_url, disposition, latitude, longitude, resolved_address,
    state, county, fips, confidence, match_method, parcel_id, parcel_geometry,
    parcel_match_type, parcel_match_distance_m, parcel_source, parcel_unavailable,
    candidates, reason, hint, evidence, fetched_at
)
VALUES (
    $1, $2, $3, $4, $5, $6, $7,
    $8, $9, $10, $11, $12, $13,
    $14, $15, $16, $17, $18, $19, $20,
    $21, $22, $23, $24,
    $25, $26, $27, $28, $29
)
RETURNING *;

-- name: NextFieldGeometryVersion :one
SELECT COALESCE(MAX(version), 0)::integer + 1
FROM pfas.field_geometry_versions
WHERE field_id = $1;

-- name: GetFieldGeometryByHash :one
SELECT id, field_id, workspace_id, version, source, source_lookup_id,
       extensions.ST_AsGeoJSON(geometry)::text AS geometry_geojson,
       geometry_hash, CAST(area_acres::text AS text) AS area_acres,
       confirmed_at, created_at
FROM pfas.field_geometry_versions
WHERE field_id = $1 AND geometry_hash = $2;

-- name: ValidateFieldGeometry :one
WITH candidate AS (
    SELECT extensions.ST_Multi(
        extensions.ST_SetSRID(extensions.ST_GeomFromGeoJSON(sqlc.arg(geojson)), 4326)
    ) AS geometry
)
SELECT CAST(extensions.ST_IsValid(geometry) AS boolean) AS is_valid,
       CAST(extensions.ST_IsValidReason(geometry) AS text) AS reason,
       CAST(NOT extensions.ST_IsEmpty(geometry) AS boolean) AS has_area,
       CAST(extensions.GeometryType(geometry) AS text) AS geometry_type
FROM candidate;

-- name: CreateFieldGeometryVersion :one
INSERT INTO pfas.field_geometry_versions (
    id, field_id, workspace_id, version, source, source_lookup_id,
    geometry, geometry_hash, area_acres, confirmed_at
)
VALUES (
    $1, $2, $3, $4, $5, $6,
    extensions.ST_Multi(extensions.ST_SetSRID(extensions.ST_GeomFromGeoJSON($7), 4326)),
    $8,
    extensions.ST_Area(
        extensions.ST_Multi(extensions.ST_SetSRID(extensions.ST_GeomFromGeoJSON($7), 4326))::extensions.geography
    ) / 4046.8564224,
    sqlc.narg(confirmed_at)
)
RETURNING id, field_id, workspace_id, version, source, source_lookup_id,
          extensions.ST_AsGeoJSON(geometry)::text AS geometry_geojson,
          geometry_hash, CAST(area_acres::text AS text) AS area_acres,
          confirmed_at, created_at;

-- name: SetCandidateFieldGeometry :exec
UPDATE pfas.candidate_fields
SET current_geometry_id = $2, updated_at = now()
WHERE id = $1 AND workspace_id = $3;

-- name: GetCurrentFieldGeometryConfirmation :one
SELECT confirmed_at
FROM pfas.field_geometry_versions
WHERE id = $1 AND field_id = $2;

-- name: ConfirmCurrentUploadedGeometry :execrows
UPDATE pfas.field_geometry_versions AS geometry
SET confirmed_at = COALESCE(geometry.confirmed_at, now())
FROM pfas.candidate_fields AS field
WHERE field.id = sqlc.arg(field_id)
  AND field.workspace_id = sqlc.arg(workspace_id)
  AND geometry.id = field.current_geometry_id
  AND geometry.field_id = field.id
  AND geometry.source = 'UPLOADED_GEOJSON';

-- name: ConfirmFieldGeometryVersion :execrows
UPDATE pfas.field_geometry_versions
SET confirmed_at = COALESCE(confirmed_at, now())
WHERE id = $1 AND field_id = $2 AND workspace_id = $3;

-- name: UpdateCandidateFieldDetails :execrows
UPDATE pfas.candidate_fields
SET mienviro_site_id = $3,
    rmp_approved = $4,
    rmp_document_reference = $5,
    usable_acres = $6,
    crop_or_use = $7,
    agronomic_rate_dry_tons_acre = $8,
    prior_loading_dry_tons = $9,
    known_constraints = $10,
    access_constraints = $11,
    updated_at = now()
WHERE id = $1 AND workspace_id = $2;

-- name: SetCandidateFieldStatus :exec
UPDATE pfas.candidate_fields
SET status = $3, updated_at = now()
WHERE id = $1 AND workspace_id = $2;

-- name: OpenFieldGap :exec
INSERT INTO pfas.field_data_gaps (id, field_id, code, detail, resolution, status)
VALUES ($1, $2, $3, $4, $5, 'OPEN')
ON CONFLICT (field_id, code) DO UPDATE
SET detail = EXCLUDED.detail,
    resolution = EXCLUDED.resolution,
    status = 'OPEN',
    resolved_at = NULL;

-- name: ResolveFieldGap :exec
UPDATE pfas.field_data_gaps
SET status = 'RESOLVED', resolved_at = now()
WHERE field_id = $1 AND code = $2 AND status = 'OPEN';

-- name: ListOpenFieldGaps :many
SELECT id, code, detail, resolution, created_at
FROM pfas.field_data_gaps
WHERE field_id = $1 AND status = 'OPEN'
ORDER BY created_at, code;

-- name: ListOpenFieldGapsForWorkspace :many
SELECT gap.id, gap.field_id, gap.code, gap.detail, gap.resolution, gap.created_at
FROM pfas.field_data_gaps gap
JOIN pfas.candidate_fields field ON field.id = gap.field_id
WHERE field.workspace_id = $1 AND gap.status = 'OPEN'
ORDER BY gap.field_id, gap.created_at, gap.code;
