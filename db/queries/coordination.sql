-- name: CreateCoordinationWorkflow :one
INSERT INTO pfas.coordination_workflows (id, workspace_id, batch_id, field_id, status, created_by_party_id)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetCoordinationWorkflow :one
SELECT w.*,
    cb.name AS created_by_name,
    COALESCE(f.name, '') AS field_name
FROM pfas.coordination_workflows w
JOIN pfas.parties cb ON cb.id = w.created_by_party_id AND cb.workspace_id = w.workspace_id
LEFT JOIN pfas.candidate_fields f ON f.id = w.field_id AND f.workspace_id = w.workspace_id
WHERE w.id = $1 AND w.workspace_id = $2;

-- name: ListCoordinationWorkflows :many
SELECT w.*,
    cb.name AS created_by_name,
    COALESCE(f.name, '') AS field_name
FROM pfas.coordination_workflows w
JOIN pfas.parties cb ON cb.id = w.created_by_party_id AND cb.workspace_id = w.workspace_id
LEFT JOIN pfas.candidate_fields f ON f.id = w.field_id AND f.workspace_id = w.workspace_id
WHERE w.workspace_id = $1
ORDER BY w.created_at DESC;

-- name: UpdateCoordinationWorkflowStatus :one
UPDATE pfas.coordination_workflows
SET status = $3, updated_at = now()
WHERE id = $1 AND workspace_id = $2
RETURNING *;

-- name: CreateCoordinationStep :one
INSERT INTO pfas.coordination_steps (id, workspace_id, workflow_id, party_id, step_role, step_type, status)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: ListCoordinationSteps :many
SELECT s.*, p.name AS party_name, p.email AS party_email
FROM pfas.coordination_steps s
LEFT JOIN pfas.parties p ON p.id = s.party_id AND p.workspace_id = s.workspace_id
WHERE s.workflow_id = $1 AND s.workspace_id = $2
ORDER BY s.step_role, s.created_at;

-- name: GetCoordinationStep :one
SELECT s.*, p.name AS party_name
FROM pfas.coordination_steps s
LEFT JOIN pfas.parties p ON p.id = s.party_id AND p.workspace_id = s.workspace_id
WHERE s.id = $1 AND s.workspace_id = $2;

-- name: ConfirmCoordinationStep :one
UPDATE pfas.coordination_steps
SET status = 'CONFIRMED', party_id = $4, notes = $3, confirmed_at = now()
WHERE id = $1 AND workspace_id = $2 AND status = 'PENDING'
RETURNING *;

-- name: RejectCoordinationStep :one
UPDATE pfas.coordination_steps
SET status = 'REJECTED', notes = $3, confirmed_at = now()
WHERE id = $1 AND workspace_id = $2 AND status = 'PENDING'
RETURNING *;

-- name: CreateCoordinationDocument :one
INSERT INTO pfas.coordination_documents (id, workspace_id, workflow_id, party_id, doc_type, filename, file_hash, mime_type, size_bytes)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: ListCoordinationDocuments :many
SELECT d.*, p.name AS party_name
FROM pfas.coordination_documents d
JOIN pfas.parties p ON p.id = d.party_id AND p.workspace_id = d.workspace_id
WHERE d.workflow_id = $1 AND d.workspace_id = $2
ORDER BY d.created_at DESC;

-- name: CreateCoordinationNotification :one
INSERT INTO pfas.coordination_notifications (id, workspace_id, workflow_id, recipient_party_id, event_type, message)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListCoordinationNotifications :many
SELECT n.*, p.name AS recipient_name
FROM pfas.coordination_notifications n
JOIN pfas.parties p ON p.id = n.recipient_party_id AND p.workspace_id = n.workspace_id
WHERE n.recipient_party_id = $1 AND n.workspace_id = $2
ORDER BY n.created_at DESC;

-- name: MarkNotificationRead :exec
UPDATE pfas.coordination_notifications
SET read_at = now()
WHERE id = $1 AND workspace_id = $2 AND read_at IS NULL;
