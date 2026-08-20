-- name: CreateJudgeDemoRun :one
INSERT INTO pfas.judge_demo_runs (
    id, idempotency_key, status, fixture_version, record, decision_hash, package_artifact, created_at, completed_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9
)
ON CONFLICT (idempotency_key) DO NOTHING
RETURNING *;

-- name: GetJudgeDemoRun :one
SELECT * FROM pfas.judge_demo_runs WHERE id = $1;

-- name: GetJudgeDemoRunByIdempotencyKey :one
SELECT * FROM pfas.judge_demo_runs WHERE idempotency_key = $1;
