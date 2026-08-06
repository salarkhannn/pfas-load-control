-- name: UpsertWorkspace :one
INSERT INTO pfas.workspaces (id, key_hash)
VALUES ($1, $2)
ON CONFLICT (key_hash) DO UPDATE SET last_seen_at = now()
RETURNING *;

-- name: GetWorkspaceByHash :one
SELECT * FROM pfas.workspaces WHERE key_hash = $1;

-- name: UpsertFacility :one
INSERT INTO pfas.facilities (id, workspace_id, name, normalized_name)
VALUES ($1, $2, $3, $4)
ON CONFLICT (workspace_id, normalized_name) DO UPDATE
SET name = EXCLUDED.name, updated_at = now()
RETURNING *;

-- name: UpsertBatch :one
INSERT INTO pfas.biosolids_batches (
    id, workspace_id, facility_id, identifier, normalized_identifier,
    wet_mass_kg, percent_solids
)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (facility_id, normalized_identifier) DO UPDATE
SET identifier = EXCLUDED.identifier,
    wet_mass_kg = COALESCE(EXCLUDED.wet_mass_kg, pfas.biosolids_batches.wet_mass_kg),
    percent_solids = COALESCE(EXCLUDED.percent_solids, pfas.biosolids_batches.percent_solids),
    updated_at = now()
RETURNING *;

-- name: ListFacilitiesForWorkspace :many
SELECT id, name, jurisdiction
FROM pfas.facilities
WHERE workspace_id = $1
ORDER BY name, id;

-- name: ListBatchesForWorkspace :many
SELECT b.id, b.identifier, CAST(COALESCE(b.wet_mass_kg::text, '') AS text) AS wet_mass_kg,
       CAST(COALESCE(b.percent_solids::text, '') AS text) AS percent_solids, b.facility_id,
       f.name AS facility_name, f.jurisdiction
FROM pfas.biosolids_batches b
JOIN pfas.facilities f ON f.id = b.facility_id AND f.workspace_id = b.workspace_id
WHERE b.workspace_id = $1
ORDER BY b.created_at DESC, b.id;

-- name: CreateLabReport :one
INSERT INTO pfas.lab_reports (
    id, workspace_id, facility_id, batch_id, original_filename,
    media_type, size_bytes, sha256, content, status
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'UPLOADED')
ON CONFLICT (workspace_id, sha256) DO NOTHING
RETURNING *;

-- name: GetLabReportByHash :one
SELECT * FROM pfas.lab_reports
WHERE workspace_id = $1 AND sha256 = $2;

-- name: RetryFailedLabReport :execrows
UPDATE pfas.lab_reports
SET status = 'UPLOADED', failure_code = NULL, updated_at = now()
WHERE id = $1 AND workspace_id = $2 AND status = 'FAILED';

-- name: GetLabReportForWorkspace :one
SELECT r.id, r.status, r.original_filename, r.media_type, r.size_bytes, r.sha256,
       r.current_version, r.failure_code, r.created_at, r.updated_at, r.confirmed_at,
       f.id AS facility_id, f.name AS facility_name, f.jurisdiction,
       b.id AS batch_id, b.identifier AS batch_identifier,
       CAST(COALESCE(b.wet_mass_kg::text, '') AS text) AS wet_mass_kg,
       CAST(COALESCE(b.percent_solids::text, '') AS text) AS percent_solids
FROM pfas.lab_reports r
JOIN pfas.facilities f ON f.id = r.facility_id AND f.workspace_id = r.workspace_id
JOIN pfas.biosolids_batches b ON b.id = r.batch_id AND b.workspace_id = r.workspace_id
WHERE r.id = $1 AND r.workspace_id = $2;

-- name: GetLabReportContent :one
SELECT original_filename, media_type, content
FROM pfas.lab_reports
WHERE id = $1 AND workspace_id = $2;

-- name: GetLabReportForProcessing :one
SELECT r.*, CAST(COALESCE(b.percent_solids::text, '') AS text) AS percent_solids
FROM pfas.lab_reports r
JOIN pfas.biosolids_batches b ON b.id = r.batch_id AND b.workspace_id = r.workspace_id
WHERE r.id = $1
FOR UPDATE OF r;

-- name: MarkLabReportProcessing :execrows
UPDATE pfas.lab_reports
SET status = 'PROCESSING', failure_code = NULL, updated_at = now()
WHERE id = $1 AND status IN ('UPLOADED', 'PROCESSING');

-- name: UpsertLabReportPage :exec
INSERT INTO pfas.lab_report_pages (
    report_id, page_number, extracted_text, extraction_method, width, height
)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (report_id, page_number) DO UPDATE
SET extracted_text = EXCLUDED.extracted_text,
    extraction_method = EXCLUDED.extraction_method,
    width = EXCLUDED.width,
    height = EXCLUDED.height;

-- name: DeleteLabReportPages :exec
DELETE FROM pfas.lab_report_pages WHERE report_id = $1;

-- name: CreateLabReportVersion :one
INSERT INTO pfas.lab_report_versions (
    id, report_id, version, status, source, laboratory, sample_identifier,
    collection_date, matrix, method, basis
)
VALUES ($1, $2, $3, 'DRAFT', $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: CreateAnalyteResult :one
INSERT INTO pfas.analyte_results (
    id, report_id, version_id, canonical_analyte, reported_analyte,
    result_text, value, unit, basis, qualifier, is_non_detect,
    reporting_limit, detection_limit, normalized_value_ug_kg_dry,
    normalized_reporting_limit_ug_kg_dry, normalized_detection_limit_ug_kg_dry,
    source_page, source_excerpt, source_bounds
)
VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8, $9, $10, $11,
    $12, $13, $14, $15, $16,
    $17, $18, $19
)
RETURNING *;

-- name: CreateLabReportGap :one
INSERT INTO pfas.lab_report_gaps (
    id, report_id, version_id, code, field_name, detail, resolution, status
)
VALUES ($1, $2, $3, $4, $5, $6, $7, 'OPEN')
RETURNING *;

-- name: CompleteLabReportExtraction :exec
UPDATE pfas.lab_reports
SET status = $2, current_version = $3, failure_code = NULL, updated_at = now()
WHERE id = $1;

-- name: FailLabReport :exec
UPDATE pfas.lab_reports
SET status = 'FAILED', failure_code = $2, updated_at = now()
WHERE id = $1 AND status <> 'CONFIRMED';

-- name: ListLabReportPages :many
SELECT page_number, extracted_text, extraction_method,
       CAST(COALESCE(width::text, '') AS text) AS width,
       CAST(COALESCE(height::text, '') AS text) AS height
FROM pfas.lab_report_pages
WHERE report_id = $1
ORDER BY page_number;

-- name: GetCurrentLabReportVersion :one
SELECT id, report_id, version, status, source, laboratory, sample_identifier,
       CAST(COALESCE(collection_date::text, '') AS text) AS collection_date,
       matrix, method, basis,
       created_at, confirmed_at
FROM pfas.lab_report_versions
WHERE report_id = $1 AND version = $2;

-- name: ListAnalytesForVersion :many
SELECT id, canonical_analyte, reported_analyte, result_text,
       CAST(COALESCE(value::text, '') AS text) AS value,
       unit, basis, qualifier, is_non_detect,
       CAST(COALESCE(reporting_limit::text, '') AS text) AS reporting_limit,
       CAST(COALESCE(detection_limit::text, '') AS text) AS detection_limit,
       CAST(COALESCE(normalized_value_ug_kg_dry::text, '') AS text) AS normalized_value_ug_kg_dry,
       CAST(COALESCE(normalized_reporting_limit_ug_kg_dry::text, '') AS text) AS normalized_reporting_limit_ug_kg_dry,
       CAST(COALESCE(normalized_detection_limit_ug_kg_dry::text, '') AS text) AS normalized_detection_limit_ug_kg_dry,
       source_page, source_excerpt, source_bounds
FROM pfas.analyte_results
WHERE version_id = $1
ORDER BY canonical_analyte DESC;

-- name: ListGapsForVersion :many
SELECT id, code, field_name, detail, resolution, status, created_at, resolved_at
FROM pfas.lab_report_gaps
WHERE version_id = $1
ORDER BY created_at, code;

-- name: SupersedeCurrentLabReportVersion :exec
UPDATE pfas.lab_report_versions
SET status = 'SUPERSEDED'
WHERE report_id = $1 AND version = $2 AND status IN ('DRAFT', 'CONFIRMED');

-- name: ConfirmLabReportVersion :execrows
UPDATE pfas.lab_report_versions
SET status = 'CONFIRMED', confirmed_at = now()
WHERE id = $1 AND status = 'DRAFT';

-- name: ConfirmLabReport :exec
UPDATE pfas.lab_reports
SET status = 'CONFIRMED', confirmed_at = now(), updated_at = now()
WHERE id = $1;

-- name: CreateLabConfirmation :one
INSERT INTO pfas.lab_confirmations (
    id, workspace_id, report_id, version_id, evidence_hash
)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;
