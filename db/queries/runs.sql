-- name: CreateRun :one
INSERT INTO pfas.agent_runs (id, kind, status, next_step)
VALUES ($1, 'MIREYE_READINESS', 'QUEUED', 1)
RETURNING *;

-- name: CreateStep :one
INSERT INTO pfas.agent_steps (id, run_id, position, tool_name, status)
VALUES ($1, $2, $3, $4, 'PENDING')
RETURNING *;

-- name: GetRun :one
SELECT * FROM pfas.agent_runs WHERE id = $1;

-- name: GetLatestRun :one
SELECT * FROM pfas.agent_runs ORDER BY created_at DESC LIMIT 1;

-- name: GetActiveRun :one
SELECT * FROM pfas.agent_runs
WHERE kind = 'MIREYE_READINESS' AND status IN ('QUEUED', 'RUNNING')
ORDER BY created_at DESC
LIMIT 1;

-- name: GetRunForUpdate :one
SELECT * FROM pfas.agent_runs WHERE id = $1 FOR UPDATE;

-- name: ListStepsForRun :many
SELECT * FROM pfas.agent_steps WHERE run_id = $1 ORDER BY position;

-- name: GetStepForUpdate :one
SELECT * FROM pfas.agent_steps WHERE run_id = $1 AND position = $2 FOR UPDATE;

-- name: MarkStepRunning :one
UPDATE pfas.agent_steps
SET status = 'RUNNING',
    attempt_count = attempt_count + 1,
    started_at = COALESCE(started_at, now()),
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: MarkRunRunning :exec
UPDATE pfas.agent_runs
SET status = 'RUNNING', started_at = COALESCE(started_at, now()), updated_at = now()
WHERE id = $1 AND status = 'QUEUED';

-- name: CompleteStep :exec
UPDATE pfas.agent_steps
SET status = 'SUCCEEDED', summary = $2, completed_at = now(), updated_at = now()
WHERE id = $1;

-- name: AdvanceRun :exec
UPDATE pfas.agent_runs
SET next_step = $2, updated_at = now()
WHERE id = $1;

-- name: CompleteRun :exec
UPDATE pfas.agent_runs
SET status = 'SUCCEEDED', next_step = 4, completed_at = now(), updated_at = now()
WHERE id = $1;

-- name: FailStep :exec
UPDATE pfas.agent_steps
SET status = 'FAILED', completed_at = now(), updated_at = now()
WHERE id = $1;

-- name: WaitRunForInput :exec
UPDATE pfas.agent_runs
SET status = 'WAITING_FOR_INPUT', updated_at = now()
WHERE id = $1;

-- name: CreateToolCall :one
INSERT INTO pfas.tool_calls (
    id, run_id, step_id, attempt, status, request_method, request_path,
    request_hash, response_hash, request_id, source_url, http_status,
    duration_ms, credit_cost, response_body, error_code, fetched_at
)
VALUES (
    $1, $2, $3, $4, $5, $6, $7,
    $8, $9, $10, $11, $12,
    $13, 0, $14, $15, $16
)
RETURNING *;

-- name: ListToolCallsForRun :many
SELECT id, run_id, step_id, attempt, status, request_method, request_path,
       request_hash, response_hash, request_id, source_url, http_status,
       duration_ms, credit_cost, error_code, fetched_at, created_at
FROM pfas.tool_calls
WHERE run_id = $1
ORDER BY created_at;

-- name: CreateDataGap :one
INSERT INTO pfas.data_gaps (id, run_id, step_id, code, detail, resolution, status)
VALUES ($1, $2, $3, $4, $5, $6, 'OPEN')
ON CONFLICT (run_id, step_id, code) DO UPDATE
SET detail = EXCLUDED.detail, resolution = EXCLUDED.resolution
RETURNING *;

-- name: ListDataGapsForRun :many
SELECT * FROM pfas.data_gaps WHERE run_id = $1 ORDER BY created_at;
