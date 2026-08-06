-- name: CreateAction :one
INSERT INTO pfas.actions (
    id, workspace_id, decision_package_id, position, code, category, title, detail,
    timing, source_id, execution_mode, status, approval_required, channel,
    recipient, subject, message, attachments, revision, payload_hash, created_at, updated_at
)
VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8,
    $9, $10, $11, $12, $13, $14,
    $15, $16, $17, $18, $19, $20, $21, $21
)
ON CONFLICT (decision_package_id, code) DO NOTHING
RETURNING *;

-- name: GetActionByPackageCode :one
SELECT * FROM pfas.actions
WHERE decision_package_id = $1 AND code = $2;

-- name: ListActionsForPackage :many
SELECT * FROM pfas.actions
WHERE decision_package_id = $1 AND workspace_id = $2
ORDER BY position;

-- name: GetActionForUpdate :one
SELECT * FROM pfas.actions
WHERE id = $1 AND workspace_id = $2
FOR UPDATE;

-- name: UpdateActionPayload :one
UPDATE pfas.actions
SET recipient = $3,
    subject = $4,
    message = $5,
    attachments = $6,
    revision = revision + 1,
    payload_hash = $7,
    status = 'PROPOSED',
    updated_at = $8
WHERE id = $1 AND workspace_id = $2
RETURNING *;

-- name: UpdateActionStatus :one
UPDATE pfas.actions
SET status = $3, updated_at = $4
WHERE id = $1 AND workspace_id = $2
RETURNING *;

-- name: CreateActionDecision :one
INSERT INTO pfas.action_decisions (
    id, workspace_id, action_id, decision_package_id, kind, action_revision,
    payload_hash, actor_name, actor_role, note, acknowledged_gap_codes, created_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING *;

-- name: ListActionDecisionsForPackage :many
SELECT * FROM pfas.action_decisions
WHERE decision_package_id = $1 AND workspace_id = $2
ORDER BY created_at, id;

-- name: GetCurrentApproval :one
SELECT * FROM pfas.action_decisions
WHERE action_id = $1
  AND workspace_id = $2
  AND kind = 'APPROVED'
  AND action_revision = $3
  AND payload_hash = $4;

-- name: GetExecutionByIdempotencyKey :one
SELECT * FROM pfas.execution_attempts
WHERE action_id = $1 AND idempotency_key = $2;

-- name: CreateExecutionAttempt :one
INSERT INTO pfas.execution_attempts (
    id, workspace_id, action_id, approval_id, idempotency_key, outcome,
    receipt, handoff_artifact, handoff_sha256, completed_at, created_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $10)
ON CONFLICT (action_id, idempotency_key) DO NOTHING
RETURNING *;

-- name: ListExecutionsForPackage :many
SELECT execution.*
FROM pfas.execution_attempts execution
JOIN pfas.actions action ON action.id = execution.action_id
WHERE action.decision_package_id = $1 AND execution.workspace_id = $2
ORDER BY execution.created_at, execution.id;

-- name: GetExecutionAttempt :one
SELECT * FROM pfas.execution_attempts
WHERE id = $1 AND workspace_id = $2;

-- name: CreatePlacementRelease :one
INSERT INTO pfas.placement_releases (
    id, workspace_id, decision_package_id, placement_evaluation_id,
    action_id, approval_id, execution_attempt_id, released_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (placement_evaluation_id) DO NOTHING
RETURNING *;

-- name: GetPlacementRelease :one
SELECT * FROM pfas.placement_releases
WHERE placement_evaluation_id = $1 AND workspace_id = $2;
