-- name: InsertPolicyRulePack :execrows
INSERT INTO pfas.policy_rule_packs (
    id, code, version, jurisdiction, authority_type, effective_from,
    retrieved_at, source_url, source_title, review_status,
    reviewed_by, reviewed_at, checksum, explanation, definition
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
ON CONFLICT (code, version) DO NOTHING;

-- name: GetPolicyRulePackByCodeVersion :one
SELECT * FROM pfas.policy_rule_packs
WHERE code = $1 AND version = $2;

-- name: GetPolicyRulePackByID :one
SELECT * FROM pfas.policy_rule_packs WHERE id = $1;

-- name: ListApplicablePolicyRulePacks :many
SELECT * FROM pfas.policy_rule_packs
WHERE jurisdiction = $1
  AND review_status = 'ACTIVE'
  AND effective_from <= $2
  AND (effective_to IS NULL OR effective_to >= $2)
ORDER BY effective_from DESC, version DESC;

-- name: GetConfirmedReportForClassification :one
SELECT r.id AS report_id, r.workspace_id, r.status AS report_status,
       r.current_version, v.id AS report_version_id, v.status AS version_status,
       v.matrix, v.method, v.basis, f.name AS facility_name, f.jurisdiction,
       b.identifier AS batch_identifier
FROM pfas.lab_reports r
JOIN pfas.lab_report_versions v
  ON v.report_id = r.id AND v.version = r.current_version
JOIN pfas.facilities f
  ON f.id = r.facility_id AND f.workspace_id = r.workspace_id
JOIN pfas.biosolids_batches b
  ON b.id = r.batch_id AND b.workspace_id = r.workspace_id
WHERE r.id = $1 AND r.workspace_id = $2;

-- name: ListConfirmedAnalytesForClassification :many
SELECT canonical_analyte, result_text, is_non_detect,
       CAST(COALESCE(normalized_value_ug_kg_dry::text, '') AS text) AS normalized_value_ug_kg_dry,
       CAST(COALESCE(normalized_reporting_limit_ug_kg_dry::text,
                     normalized_detection_limit_ug_kg_dry::text, '') AS text) AS upper_bound_ug_kg_dry,
       source_page
FROM pfas.analyte_results
WHERE report_id = $1 AND version_id = $2
ORDER BY canonical_analyte;

-- name: CreateBatchPolicyDecision :one
INSERT INTO pfas.batch_policy_decisions (
    id, workspace_id, report_id, report_version_id, rule_pack_id,
    jurisdiction, tier, matched_rule_id, explanation, blocking_reason,
    input_hash, analyte_evidence
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
ON CONFLICT (report_version_id, rule_pack_id) DO NOTHING
RETURNING *;

-- name: GetBatchPolicyDecisionByVersion :one
SELECT * FROM pfas.batch_policy_decisions
WHERE report_version_id = $1 AND rule_pack_id = $2;

-- name: GetBatchPolicyDecisionForWorkspace :one
SELECT d.*, f.name AS facility_name, b.identifier AS batch_identifier,
       v.version AS report_version
FROM pfas.batch_policy_decisions d
JOIN pfas.lab_reports r ON r.id = d.report_id AND r.workspace_id = d.workspace_id
JOIN pfas.facilities f ON f.id = r.facility_id AND f.workspace_id = r.workspace_id
JOIN pfas.biosolids_batches b ON b.id = r.batch_id AND b.workspace_id = r.workspace_id
JOIN pfas.lab_report_versions v ON v.id = d.report_version_id AND v.report_id = d.report_id
JOIN pfas.policy_rule_packs p ON p.id = d.rule_pack_id
WHERE d.report_id = $1 AND d.workspace_id = $2
  AND p.review_status = 'ACTIVE'
  AND p.effective_from <= CURRENT_DATE
  AND (p.effective_to IS NULL OR p.effective_to >= CURRENT_DATE)
ORDER BY p.effective_from DESC, p.version DESC
LIMIT 1;

-- name: CreateBatchPolicyRequirement :exec
INSERT INTO pfas.batch_policy_requirements (
    id, decision_id, position, requirement_id, title, detail, timing,
    rule_id, source_url, source_title, authority_type
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11);

-- name: ListBatchPolicyRequirements :many
SELECT * FROM pfas.batch_policy_requirements
WHERE decision_id = $1
ORDER BY position;
