-- name: CreateRegistryEntry :one
INSERT INTO pfas.registry_entries (id, workspace_id, party_id, entry_type, name, data, latitude, longitude, search_vector)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, to_tsvector('english', $5))
RETURNING *;

-- name: GetRegistryEntry :one
SELECT * FROM pfas.registry_entries
WHERE id = $1 AND workspace_id = $2;

-- name: ListRegistryEntries :many
SELECT * FROM pfas.registry_entries
WHERE workspace_id = $1 AND ($2::text = '' OR entry_type = $2)
ORDER BY updated_at DESC;

-- name: SearchRegistryEntries :many
SELECT *, ts_rank(search_vector, plainto_tsquery('english', $2)) AS rank
FROM pfas.registry_entries
WHERE workspace_id = $1
  AND search_vector @@ plainto_tsquery('english', $2)
ORDER BY rank DESC;

-- name: UpdateRegistryEntrySearchVector :exec
UPDATE pfas.registry_entries
SET search_vector = to_tsvector('english', name), updated_at = now()
WHERE id = $1 AND workspace_id = $2;

-- name: FindNearbyRegistryEntries :many
SELECT *,
    ST_Distance(
        ST_SetSRID(ST_MakePoint(COALESCE(longitude, 0), COALESCE(latitude, 0)), 4326)::geography,
        ST_SetSRID(ST_MakePoint($3, $2), 4326)::geography
    ) AS distance_m
FROM pfas.registry_entries
WHERE workspace_id = $1
  AND latitude IS NOT NULL AND longitude IS NOT NULL
  AND entry_type = $4
ORDER BY distance_m ASC
LIMIT $5;

-- name: DeleteRegistryEntry :exec
DELETE FROM pfas.registry_entries
WHERE id = $1 AND workspace_id = $2;

-- name: CreateRegistryMatch :one
INSERT INTO pfas.registry_matches (id, workspace_id, plant_entry_id, field_entry_id, score, reasons)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListRegistryMatchesByPlant :many
SELECT rm.*, re.name AS field_name, re.data AS field_data
FROM pfas.registry_matches rm
JOIN pfas.registry_entries re ON re.id = rm.field_entry_id AND re.workspace_id = rm.workspace_id
WHERE rm.plant_entry_id = $1 AND rm.workspace_id = $2
ORDER BY rm.score DESC;

-- name: DeleteRegistryMatchesByPlant :exec
DELETE FROM pfas.registry_matches
WHERE plant_entry_id = $1 AND workspace_id = $2;
