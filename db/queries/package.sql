-- name: GetDecisionPackageContext :one
SELECT decision.id AS decision_id, decision.workspace_id, decision.report_id,
       decision.tier, decision.input_hash AS decision_input_hash
FROM pfas.batch_policy_decisions decision
WHERE decision.id = $1 AND decision.workspace_id = $2;

-- name: CreateDecisionPackage :one
INSERT INTO pfas.decision_packages (
    id, workspace_id, decision_id, placement_evaluation_id, response_run_id,
    schema_version, status, input_hash, snapshot, evidence_ledger,
    proposed_actions, json_artifact, html_artifact, pdf_artifact,
    json_sha256, html_sha256, pdf_sha256, created_at
)
VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8, $9, $10,
    $11, $12, $13, $14,
    $15, $16, $17, $18
)
ON CONFLICT (decision_id, input_hash) DO NOTHING
RETURNING *;

-- name: GetDecisionPackageByInput :one
SELECT * FROM pfas.decision_packages
WHERE decision_id = $1 AND input_hash = $2;

-- name: GetLatestDecisionPackage :one
SELECT * FROM pfas.decision_packages
WHERE decision_id = $1 AND workspace_id = $2
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: GetDecisionPackage :one
SELECT * FROM pfas.decision_packages
WHERE id = $1 AND workspace_id = $2;
