-- name: CreateApplicationRecord :one
INSERT INTO pfas.application_records (id, workspace_id, batch_id, field_id, contractor_party_id, application_date, dry_tons, rate_dry_tons_per_acre, acres_applied, weather_conditions, field_condition_notes)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;

-- name: GetApplicationRecord :one
SELECT ar.*, f.name AS field_name, p.name AS contractor_name
FROM pfas.application_records ar
JOIN pfas.candidate_fields f ON f.id = ar.field_id AND f.workspace_id = ar.workspace_id
JOIN pfas.parties p ON p.id = ar.contractor_party_id AND p.workspace_id = ar.workspace_id
WHERE ar.id = $1 AND ar.workspace_id = $2;

-- name: ListApplicationRecordsByField :many
SELECT ar.*, p.name AS contractor_name
FROM pfas.application_records ar
JOIN pfas.parties p ON p.id = ar.contractor_party_id AND p.workspace_id = ar.workspace_id
WHERE ar.field_id = $1 AND ar.workspace_id = $2
ORDER BY ar.application_date DESC;

-- name: ListApplicationRecordsByContractor :many
SELECT ar.*, f.name AS field_name
FROM pfas.application_records ar
JOIN pfas.candidate_fields f ON f.id = ar.field_id AND f.workspace_id = ar.workspace_id
WHERE ar.contractor_party_id = $1 AND ar.workspace_id = $2
ORDER BY ar.application_date DESC;

-- name: UpsertFieldLoadingLedger :one
INSERT INTO pfas.field_loading_ledger (id, workspace_id, field_id, year, cumulative_dry_tons, last_application_date, last_updated)
VALUES ($1, $2, $3, $4, $5, $6, now())
ON CONFLICT (field_id, year, workspace_id) DO UPDATE
SET cumulative_dry_tons = pfas.field_loading_ledger.cumulative_dry_tons + $5,
    last_application_date = GREATEST(pfas.field_loading_ledger.last_application_date, $6),
    last_updated = now()
RETURNING *;

-- name: GetFieldLoadingLedger :one
SELECT * FROM pfas.field_loading_ledger
WHERE field_id = $1 AND year = $2 AND workspace_id = $3;

-- name: ListFieldLoadingLedgerByField :many
SELECT * FROM pfas.field_loading_ledger
WHERE field_id = $1 AND workspace_id = $2
ORDER BY year DESC;

-- name: CreateApplicationConfirmation :one
INSERT INTO pfas.application_confirmations (id, workspace_id, application_id, farmer_party_id, confirmed, notes)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ConfirmApplication :one
UPDATE pfas.application_confirmations
SET confirmed = true, notes = $3, confirmed_at = now()
WHERE id = $1 AND workspace_id = $2 AND confirmed = false
RETURNING *;

-- name: GetApplicationConfirmation :one
SELECT ac.*, p.name AS farmer_name
FROM pfas.application_confirmations ac
JOIN pfas.parties p ON p.id = ac.farmer_party_id AND p.workspace_id = ac.workspace_id
WHERE ac.id = $1 AND ac.workspace_id = $2;
