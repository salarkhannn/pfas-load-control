-- name: GetActivePhysicalEvaluation :one
SELECT * FROM pfas.physical_evaluations
WHERE field_id = $1 AND workspace_id = $2 AND status IN ('QUEUED', 'RUNNING')
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: CreatePhysicalEvaluation :one
INSERT INTO pfas.physical_evaluations (
    id, workspace_id, field_id, geometry_id, status, field_set_version, aggregation_version
)
VALUES ($1, $2, $3, $4, 'QUEUED', $5, $6)
RETURNING *;

-- name: GetPhysicalEvaluation :one
SELECT * FROM pfas.physical_evaluations
WHERE id = $1 AND workspace_id = $2;

-- name: GetLatestPhysicalEvaluation :one
SELECT * FROM pfas.physical_evaluations
WHERE field_id = $1 AND workspace_id = $2
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: GetPhysicalEvaluationForWork :one
SELECT evaluation.*, field.status AS field_status, field.name AS field_name,
       facility.name AS facility_name, geometry.version AS geometry_version,
       geometry.geometry_hash,
       extensions.ST_AsGeoJSON(geometry.geometry)::text AS geometry_geojson
FROM pfas.physical_evaluations evaluation
JOIN pfas.candidate_fields field
  ON field.id = evaluation.field_id AND field.workspace_id = evaluation.workspace_id
JOIN pfas.facilities facility
  ON facility.id = field.facility_id AND facility.workspace_id = field.workspace_id
JOIN pfas.field_geometry_versions geometry
  ON geometry.id = evaluation.geometry_id AND geometry.field_id = evaluation.field_id
WHERE evaluation.id = $1
;

-- name: MarkPhysicalEvaluationRunning :exec
UPDATE pfas.physical_evaluations
SET status = 'RUNNING', started_at = COALESCE(started_at, now()), updated_at = now(),
    failure_code = NULL, failure_detail = NULL
WHERE id = $1;

-- name: SetPhysicalEvaluationGuard :exec
UPDATE pfas.physical_evaluations
SET catalog_version = $2, catalog_etag = $3, sample_count = $4,
    projected_credits = $5, request_hash = $6, updated_at = now()
WHERE id = $1;

-- name: CompletePhysicalEvaluation :exec
UPDATE pfas.physical_evaluations
SET status = $2, completed_at = now(), updated_at = now(),
    failure_code = NULL, failure_detail = NULL
WHERE id = $1;

-- name: FailPhysicalEvaluation :exec
UPDATE pfas.physical_evaluations
SET status = $2, failure_code = $3, failure_detail = $4,
    completed_at = now(), updated_at = now()
WHERE id = $1;

-- name: DeletePhysicalEvaluationEvidence :exec
DELETE FROM pfas.physical_sample_points WHERE evaluation_id = $1;

-- name: GeneratePhysicalSamplePoints :many
WITH target AS (
    SELECT geometry
    FROM pfas.field_geometry_versions
    WHERE id = $1 AND field_id = $2
), bounds AS (
    SELECT geometry, extensions.ST_XMin(geometry) AS xmin, extensions.ST_XMax(geometry) AS xmax,
           extensions.ST_YMin(geometry) AS ymin, extensions.ST_YMax(geometry) AS ymax
    FROM target
), cells AS (
    SELECT row_number() OVER (ORDER BY y, x)::integer AS ordinal,
           extensions.ST_Intersection(
             geometry,
             extensions.ST_MakeEnvelope(
               xmin + (x * (xmax - xmin) / 2.0),
               ymin + (y * (ymax - ymin) / 2.0),
               xmin + ((x + 1) * (xmax - xmin) / 2.0),
               ymin + ((y + 1) * (ymax - ymin) / 2.0),
               4326
             )
           ) AS part
    FROM bounds CROSS JOIN generate_series(0, 1) AS x CROSS JOIN generate_series(0, 1) AS y
), candidates AS (
    SELECT 0 AS priority, 'Interior anchor'::text AS label,
           extensions.ST_PointOnSurface(geometry) AS point
    FROM target
    UNION ALL
    SELECT ordinal AS priority, 'Area ' || ordinal::text AS label,
           extensions.ST_PointOnSurface(part) AS point
    FROM cells
    WHERE NOT extensions.ST_IsEmpty(part) AND extensions.ST_Area(part::extensions.geography) > 1
), distinct_points AS (
    SELECT DISTINCT ON (extensions.ST_AsBinary(point)) priority, label, point
    FROM candidates
    ORDER BY extensions.ST_AsBinary(point), priority
), ordered AS (
    SELECT row_number() OVER (ORDER BY priority, label)::integer - 1 AS sample_index, label, point
    FROM distinct_points
)
INSERT INTO pfas.physical_sample_points (evaluation_id, sample_index, label, point)
SELECT $3, sample_index, label, point FROM ordered
WHERE sample_index < 5
RETURNING sample_index, label, extensions.ST_Y(point)::double precision AS latitude,
          extensions.ST_X(point)::double precision AS longitude;

-- name: ListPhysicalSamplePoints :many
SELECT sample_index, label, extensions.ST_Y(point)::double precision AS latitude,
       extensions.ST_X(point)::double precision AS longitude
FROM pfas.physical_sample_points
WHERE evaluation_id = $1
ORDER BY sample_index;

-- name: CreateMireyeFetchBatch :exec
INSERT INTO pfas.mireye_fetch_batches (
    id, evaluation_id, request_hash, response_hash, request_id, source_url,
    http_status, request, response, fetched_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);

-- name: UpsertPhysicalSampleFact :exec
INSERT INTO pfas.physical_sample_facts (
    evaluation_id, sample_index, field_name, status, value, unit, source,
    source_url, confidence, fetched_at, dataset_vintage, ttl_seconds,
    notes, error, retryable
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
ON CONFLICT (evaluation_id, sample_index, field_name) DO UPDATE
SET status = EXCLUDED.status, value = EXCLUDED.value, unit = EXCLUDED.unit,
    source = EXCLUDED.source, source_url = EXCLUDED.source_url,
    confidence = EXCLUDED.confidence, fetched_at = EXCLUDED.fetched_at,
    dataset_vintage = EXCLUDED.dataset_vintage, ttl_seconds = EXCLUDED.ttl_seconds,
    notes = EXCLUDED.notes, error = EXCLUDED.error, retryable = EXCLUDED.retryable;

-- name: ListPhysicalSampleFacts :many
SELECT fact.evaluation_id, fact.sample_index, point.label AS sample_label,
       extensions.ST_Y(point.point)::double precision AS latitude,
       extensions.ST_X(point.point)::double precision AS longitude,
       fact.field_name, fact.status, fact.value, fact.unit, fact.source,
       fact.source_url, fact.confidence, fact.fetched_at, fact.dataset_vintage,
       fact.ttl_seconds, fact.notes, fact.error, fact.retryable
FROM pfas.physical_sample_facts fact
JOIN pfas.physical_sample_points point
  ON point.evaluation_id = fact.evaluation_id AND point.sample_index = fact.sample_index
WHERE fact.evaluation_id = $1
ORDER BY fact.field_name, fact.sample_index;

-- name: UpsertPhysicalFieldFact :exec
INSERT INTO pfas.physical_field_facts (
    evaluation_id, field_name, category, label, state, aggregate_method,
    value, unit, source, source_url, fetched_at, ok_count, absent_count,
    failed_count, sample_indices, critical
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
ON CONFLICT (evaluation_id, field_name) DO UPDATE
SET category = EXCLUDED.category, label = EXCLUDED.label, state = EXCLUDED.state,
    aggregate_method = EXCLUDED.aggregate_method, value = EXCLUDED.value,
    unit = EXCLUDED.unit, source = EXCLUDED.source, source_url = EXCLUDED.source_url,
    fetched_at = EXCLUDED.fetched_at, ok_count = EXCLUDED.ok_count,
    absent_count = EXCLUDED.absent_count, failed_count = EXCLUDED.failed_count,
    sample_indices = EXCLUDED.sample_indices, critical = EXCLUDED.critical;

-- name: ListPhysicalFieldFacts :many
SELECT * FROM pfas.physical_field_facts
WHERE evaluation_id = $1
ORDER BY category, field_name;

-- name: UpsertSupplementalEvidence :exec
INSERT INTO pfas.supplemental_evidence (
    evaluation_id, provider, kind, status, title, summary, value,
    source_url, source_vintage, fetched_at, caveat
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
ON CONFLICT (evaluation_id, provider, kind) DO UPDATE
SET status = EXCLUDED.status, title = EXCLUDED.title, summary = EXCLUDED.summary,
    value = EXCLUDED.value, source_url = EXCLUDED.source_url,
    source_vintage = EXCLUDED.source_vintage, fetched_at = EXCLUDED.fetched_at,
    caveat = EXCLUDED.caveat;

-- name: ListSupplementalEvidence :many
SELECT * FROM pfas.supplemental_evidence
WHERE evaluation_id = $1
ORDER BY provider, kind;

-- name: UpsertPhysicalDataGap :exec
INSERT INTO pfas.physical_data_gaps (evaluation_id, code, source, detail, critical)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (evaluation_id, code) DO UPDATE
SET source = EXCLUDED.source, detail = EXCLUDED.detail, critical = EXCLUDED.critical;

-- name: ListPhysicalDataGaps :many
SELECT * FROM pfas.physical_data_gaps
WHERE evaluation_id = $1
ORDER BY critical DESC, source, code;

-- name: DistanceFromFieldToPointMeters :one
SELECT extensions.ST_Distance(
    geometry.geometry::extensions.geography,
    extensions.ST_SetSRID(
        extensions.ST_MakePoint(sqlc.arg(longitude)::double precision, sqlc.arg(latitude)::double precision),
        4326
    )::extensions.geography
)::double precision AS distance_m
FROM pfas.field_geometry_versions geometry
WHERE geometry.id = sqlc.arg(geometry_id) AND geometry.field_id = sqlc.arg(field_id);

-- name: GetFieldGeometryEnvelope :one
SELECT extensions.ST_XMin(geometry)::double precision AS min_lng,
       extensions.ST_YMin(geometry)::double precision AS min_lat,
       extensions.ST_XMax(geometry)::double precision AS max_lng,
       extensions.ST_YMax(geometry)::double precision AS max_lat
FROM pfas.field_geometry_versions
WHERE id = $1 AND field_id = $2;
