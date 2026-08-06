-- name: GetPlacementContext :one
SELECT decision.id AS decision_id, decision.workspace_id, decision.tier,
       decision.input_hash AS decision_input_hash, decision.matched_rule_id,
       decision.rule_pack_id, pack.definition AS rule_pack_definition,
       report.batch_id, report.facility_id,
       CAST(COALESCE(batch.wet_mass_kg::text, '') AS text) AS wet_mass_kg,
       CAST(COALESCE(batch.percent_solids::text, '') AS text) AS percent_solids
FROM pfas.batch_policy_decisions decision
JOIN pfas.policy_rule_packs pack ON pack.id = decision.rule_pack_id
JOIN pfas.lab_reports report
  ON report.id = decision.report_id AND report.workspace_id = decision.workspace_id
JOIN pfas.biosolids_batches batch
  ON batch.id = report.batch_id AND batch.workspace_id = report.workspace_id
WHERE decision.id = $1 AND decision.workspace_id = $2;

-- name: UpdatePlacementBatchQuantity :execrows
UPDATE pfas.biosolids_batches
SET wet_mass_kg = $3, percent_solids = $4, updated_at = now()
WHERE id = $1 AND workspace_id = $2;

-- name: ListPlacementFieldInputs :many
SELECT field.id, field.name, field.status, COALESCE(field.rmp_approved, false) AS rmp_approved,
       CAST(COALESCE(field.usable_acres::text, '') AS text) AS usable_acres,
       CAST(COALESCE(field.agronomic_rate_dry_tons_acre::text, '') AS text) AS agronomic_rate,
       CAST(COALESCE(field.prior_loading_dry_tons::text, '') AS text) AS prior_loading_dry_tons,
       COALESCE(field.crop_or_use, '') AS crop_or_use,
       COALESCE(evaluation.id, '00000000-0000-0000-0000-000000000000'::uuid) AS physical_evaluation_id,
       COALESCE(evaluation.status, '') AS physical_status,
       COALESCE(gaps.critical_count, 0)::integer AS physical_critical_gaps,
       COALESCE(gaps.other_count, 0)::integer AS physical_other_gaps,
       COALESCE(supplemental.all_available, false) AS supplemental_available
FROM pfas.candidate_fields field
LEFT JOIN LATERAL (
    SELECT item.id, item.status
    FROM pfas.physical_evaluations item
    WHERE item.field_id = field.id AND item.workspace_id = field.workspace_id
    ORDER BY item.created_at DESC, item.id DESC
    LIMIT 1
) evaluation ON true
LEFT JOIN LATERAL (
    SELECT count(*) FILTER (WHERE gap.critical)::integer AS critical_count,
           count(*) FILTER (WHERE NOT gap.critical)::integer AS other_count
    FROM pfas.physical_data_gaps gap
    WHERE gap.evaluation_id = evaluation.id
) gaps ON true
LEFT JOIN LATERAL (
    SELECT count(*) > 0 AND bool_and(item.status = 'AVAILABLE') AS all_available
    FROM pfas.supplemental_evidence item
    WHERE item.evaluation_id = evaluation.id
) supplemental ON true
WHERE field.workspace_id = $1 AND field.facility_id = $2
ORDER BY field.created_at, field.id;

-- name: GetPlacementEvaluationByInput :one
SELECT * FROM pfas.placement_evaluations
WHERE decision_id = $1 AND input_hash = $2;

-- name: CreatePlacementEvaluation :one
INSERT INTO pfas.placement_evaluations (
    id, workspace_id, decision_id, batch_id, status, tier, config_version,
    config_checksum, input_hash, wet_mass_kg, percent_solids, batch_dry_tons,
    allocated_dry_tons, unallocated_dry_tons
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
ON CONFLICT (decision_id, input_hash) DO NOTHING
RETURNING *;

-- name: CreatePlacementFieldResult :exec
INSERT INTO pfas.placement_field_results (
    evaluation_id, field_id, physical_evaluation_id, field_name, disposition, rank,
    explanation, counterfactual, high_concern_count, moderate_concern_count,
    data_gap_count, allowed_rate_dry_tons_acre, available_capacity_dry_tons,
    road_access_distance_m, reasons
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15);

-- name: CreatePlacementVulnerabilityCategory :exec
INSERT INTO pfas.placement_vulnerability_categories (
    evaluation_id, field_id, category_key, label, band, explanation, components,
    authority_type, source_title, source_url, config_version
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11);

-- name: CreatePlacementAllocation :exec
INSERT INTO pfas.placement_allocations (
    evaluation_id, position, field_id, field_name, dry_tons, acres, rate_dry_tons_acre
)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: CreatePlacementDataGap :exec
INSERT INTO pfas.placement_data_gaps (evaluation_id, code, detail, resolution)
VALUES ($1, $2, $3, $4);

-- name: GetLatestPlacementEvaluation :one
SELECT * FROM pfas.placement_evaluations
WHERE decision_id = $1 AND workspace_id = $2 AND config_checksum = $3
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: GetPlacementEvaluation :one
SELECT * FROM pfas.placement_evaluations
WHERE id = $1 AND workspace_id = $2;

-- name: ListPlacementFieldResults :many
SELECT evaluation_id, field_id, physical_evaluation_id, field_name, disposition, rank,
       explanation, counterfactual, high_concern_count, moderate_concern_count,
       data_gap_count,
       CAST(COALESCE(allowed_rate_dry_tons_acre::text, '') AS text) AS allowed_rate,
       CAST(COALESCE(available_capacity_dry_tons::text, '') AS text) AS available_capacity,
       road_access_distance_m, reasons
FROM pfas.placement_field_results
WHERE evaluation_id = $1
ORDER BY COALESCE(rank, 2147483647), disposition, field_name;

-- name: ListPlacementVulnerabilityCategories :many
SELECT * FROM pfas.placement_vulnerability_categories
WHERE evaluation_id = $1
ORDER BY field_id,
    CASE category_key
        WHEN 'WATER_RECEPTORS' THEN 1
        WHEN 'SUBSURFACE_MOBILITY' THEN 2
        WHEN 'SURFACE_TRANSPORT' THEN 3
        WHEN 'HUMAN_FOOD_EXPOSURE' THEN 4
        ELSE 5
    END;

-- name: ListPlacementAllocations :many
SELECT evaluation_id, position, field_id, field_name,
       CAST(dry_tons::text AS text) AS dry_tons,
       CAST(acres::text AS text) AS acres,
       CAST(rate_dry_tons_acre::text AS text) AS rate
FROM pfas.placement_allocations
WHERE evaluation_id = $1
ORDER BY position;

-- name: ListPlacementDataGaps :many
SELECT * FROM pfas.placement_data_gaps
WHERE evaluation_id = $1
ORDER BY code;
