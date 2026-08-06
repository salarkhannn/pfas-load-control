-- name: GetResponseDecisionContext :one
SELECT decision.id, decision.workspace_id, decision.tier, decision.input_hash AS decision_input_hash,
       report.facility_id, facility.name AS facility_name, batch.identifier AS batch_identifier,
       pack.version AS policy_version, pack.source_url AS policy_source_url
FROM pfas.batch_policy_decisions decision
JOIN pfas.lab_reports report
  ON report.id = decision.report_id AND report.workspace_id = decision.workspace_id
JOIN pfas.facilities facility
  ON facility.id = report.facility_id AND facility.workspace_id = report.workspace_id
JOIN pfas.biosolids_batches batch
  ON batch.id = report.batch_id AND batch.workspace_id = report.workspace_id
JOIN pfas.policy_rule_packs pack ON pack.id = decision.rule_pack_id
WHERE decision.id = $1 AND decision.workspace_id = $2;

-- name: GetFacilityLocationLookupByHash :one
SELECT * FROM pfas.facility_location_lookups
WHERE facility_id = $1 AND workspace_id = $2 AND request_hash = $3;

-- name: CreateFacilityLocationLookup :one
INSERT INTO pfas.facility_location_lookups (
    id, workspace_id, facility_id, input, input_kind, request_hash, response_hash,
    request_id, source_url, disposition, latitude, longitude, resolved_address,
    state, county, confidence, match_method, candidates, reason, hint, evidence, fetched_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13,
        $14, $15, $16, $17, $18, $19, $20, $21, $22)
RETURNING *;

-- name: GetFacilityLocationLookup :one
SELECT * FROM pfas.facility_location_lookups
WHERE id = $1 AND workspace_id = $2;

-- name: GetLatestFacilityLocationLookup :one
SELECT lookup.*
FROM pfas.facility_location_lookups lookup
JOIN pfas.facilities facility
  ON facility.id = lookup.facility_id AND facility.workspace_id = lookup.workspace_id
WHERE lookup.facility_id = $1 AND lookup.workspace_id = $2
ORDER BY lookup.created_at DESC, lookup.id DESC
LIMIT 1;

-- name: ConfirmFacilityLocationLookup :one
UPDATE pfas.facility_location_lookups
SET confirmed_at = COALESCE(confirmed_at, now())
WHERE id = $1 AND workspace_id = $2 AND disposition = 'resolved'
  AND latitude IS NOT NULL AND longitude IS NOT NULL AND state = 'MI'
RETURNING *;

-- name: GetActiveResponseRun :one
SELECT * FROM pfas.response_runs
WHERE decision_id = $1 AND workspace_id = $2 AND status IN ('QUEUED', 'RUNNING')
ORDER BY created_at DESC, id DESC LIMIT 1;

-- name: CreateResponseRun :one
INSERT INTO pfas.response_runs (
    id, workspace_id, decision_id, facility_location_id, tier, status,
    input_hash, policy_source_url, policy_version
)
VALUES ($1, $2, $3, $4, $5, 'QUEUED', $6, $7, $8)
ON CONFLICT (decision_id, input_hash) DO NOTHING
RETURNING *;

-- name: GetResponseRunByInput :one
SELECT * FROM pfas.response_runs
WHERE decision_id = $1 AND input_hash = $2;

-- name: GetResponseRun :one
SELECT run.*, facility.name AS facility_name, batch.identifier AS batch_identifier,
       location.resolved_address, location.latitude, location.longitude,
       location.confidence AS location_confidence, location.source_url AS location_source_url,
       location.fetched_at AS location_fetched_at
FROM pfas.response_runs run
JOIN pfas.batch_policy_decisions decision
  ON decision.id = run.decision_id AND decision.workspace_id = run.workspace_id
JOIN pfas.lab_reports report
  ON report.id = decision.report_id AND report.workspace_id = decision.workspace_id
JOIN pfas.facilities facility
  ON facility.id = report.facility_id AND facility.workspace_id = report.workspace_id
JOIN pfas.biosolids_batches batch
  ON batch.id = report.batch_id AND batch.workspace_id = report.workspace_id
JOIN pfas.facility_location_lookups location
  ON location.id = run.facility_location_id AND location.workspace_id = run.workspace_id
WHERE run.id = $1 AND run.workspace_id = $2;

-- name: GetLatestResponseRun :one
SELECT run.*, facility.name AS facility_name, batch.identifier AS batch_identifier,
       location.resolved_address, location.latitude, location.longitude,
       location.confidence AS location_confidence, location.source_url AS location_source_url,
       location.fetched_at AS location_fetched_at
FROM pfas.response_runs run
JOIN pfas.batch_policy_decisions decision
  ON decision.id = run.decision_id AND decision.workspace_id = run.workspace_id
JOIN pfas.lab_reports report
  ON report.id = decision.report_id AND report.workspace_id = decision.workspace_id
JOIN pfas.facilities facility
  ON facility.id = report.facility_id AND facility.workspace_id = report.workspace_id
JOIN pfas.biosolids_batches batch
  ON batch.id = report.batch_id AND batch.workspace_id = report.workspace_id
JOIN pfas.facility_location_lookups location
  ON location.id = run.facility_location_id AND location.workspace_id = run.workspace_id
WHERE run.decision_id = $1 AND run.workspace_id = $2
ORDER BY run.created_at DESC, run.id DESC LIMIT 1;

-- name: GetResponseRunForWork :one
SELECT run.*, location.latitude, location.longitude, location.resolved_address,
       facility.name AS facility_name, batch.identifier AS batch_identifier
FROM pfas.response_runs run
JOIN pfas.facility_location_lookups location
  ON location.id = run.facility_location_id AND location.workspace_id = run.workspace_id
JOIN pfas.batch_policy_decisions decision
  ON decision.id = run.decision_id AND decision.workspace_id = run.workspace_id
JOIN pfas.lab_reports report
  ON report.id = decision.report_id AND report.workspace_id = decision.workspace_id
JOIN pfas.facilities facility
  ON facility.id = report.facility_id AND facility.workspace_id = report.workspace_id
JOIN pfas.biosolids_batches batch
  ON batch.id = report.batch_id AND batch.workspace_id = report.workspace_id
WHERE run.id = $1;

-- name: MarkResponseRunRunning :exec
UPDATE pfas.response_runs
SET status = 'RUNNING', started_at = COALESCE(started_at, now()), updated_at = now()
WHERE id = $1 AND status = 'QUEUED';

-- name: ClearResponseRunOutputs :exec
WITH deleted_tasks AS (DELETE FROM pfas.response_tasks WHERE run_id = sqlc.arg(run_id)),
deleted_evidence AS (DELETE FROM pfas.response_evidence WHERE run_id = sqlc.arg(run_id)),
deleted_leads AS (DELETE FROM pfas.investigation_leads WHERE run_id = sqlc.arg(run_id)),
deleted_candidates AS (DELETE FROM pfas.alternative_management_candidates WHERE run_id = sqlc.arg(run_id))
DELETE FROM pfas.response_data_gaps WHERE response_data_gaps.run_id = sqlc.arg(run_id);

-- name: CompleteResponseRun :exec
UPDATE pfas.response_runs
SET status = $2, completed_at = now(), updated_at = now()
WHERE id = $1;

-- name: FailResponseRun :exec
UPDATE pfas.response_runs
SET status = 'FAILED', failure_code = $2, failure_detail = $3,
    completed_at = now(), updated_at = now()
WHERE id = $1;

-- name: CreateResponseTask :exec
INSERT INTO pfas.response_tasks (run_id, position, code, category, title, detail, timing, state)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: ListResponseTasks :many
SELECT * FROM pfas.response_tasks WHERE run_id = $1 ORDER BY position;

-- name: CreateResponseEvidence :exec
INSERT INTO pfas.response_evidence (
    id, run_id, provider, kind, status, title, summary, data, source_url,
    source_vintage, request_hash, response_hash, request_id, fetched_at, caveat
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15);

-- name: ListResponseEvidence :many
SELECT * FROM pfas.response_evidence WHERE run_id = $1 ORDER BY provider, kind;

-- name: CreateInvestigationLead :exec
INSERT INTO pfas.investigation_leads (
    run_id, position, registry_id, facility_name, city, state, naics_codes,
    evidence_tier, evidence_label, rationale, caveat, source_url
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12);

-- name: ListInvestigationLeads :many
SELECT * FROM pfas.investigation_leads WHERE run_id = $1 ORDER BY position;

-- name: CreateAlternativeCandidate :exec
INSERT INTO pfas.alternative_management_candidates (
    run_id, position, wds_id, facility_name, facility_type, address, city, county,
    latitude, longitude, disposal_area_status, straightline_distance_km,
    route_status, driving_distance_km, duration_minutes, route_note, source_url
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17);

-- name: ListAlternativeCandidates :many
SELECT * FROM pfas.alternative_management_candidates WHERE run_id = $1 ORDER BY position;

-- name: CreateResponseDataGap :exec
INSERT INTO pfas.response_data_gaps (run_id, code, detail, resolution, critical)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (run_id, code) DO UPDATE
SET detail = EXCLUDED.detail, resolution = EXCLUDED.resolution, critical = EXCLUDED.critical;

-- name: ListResponseDataGaps :many
SELECT * FROM pfas.response_data_gaps WHERE run_id = $1 ORDER BY critical DESC, code;
