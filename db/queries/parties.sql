-- name: CreateParty :one
INSERT INTO pfas.parties (id, workspace_id, role, name, email, phone, latitude, longitude)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetParty :one
SELECT * FROM pfas.parties
WHERE id = $1 AND workspace_id = $2;

-- name: ListPartiesByWorkspace :many
SELECT * FROM pfas.parties
WHERE workspace_id = $1
ORDER BY created_at DESC;

-- name: ListPartiesByRole :many
SELECT * FROM pfas.parties
WHERE workspace_id = $1 AND role = $2
ORDER BY created_at DESC;

-- name: UpdateParty :one
UPDATE pfas.parties
SET name = $3, email = $4, phone = $5, latitude = $6, longitude = $7, updated_at = now()
WHERE id = $1 AND workspace_id = $2
RETURNING *;

-- name: DeleteParty :exec
DELETE FROM pfas.parties
WHERE id = $1 AND workspace_id = $2;

-- name: CreateConsent :one
INSERT INTO pfas.party_consents (id, workspace_id, granter_party_id, grantee_party_id, scope, purpose, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetConsent :one
SELECT * FROM pfas.party_consents
WHERE id = $1 AND workspace_id = $2;

-- name: ListConsentsByGranter :many
SELECT c.*, gt.name AS granter_name, gg.name AS grantee_name
FROM pfas.party_consents c
JOIN pfas.parties gt ON gt.id = c.granter_party_id AND gt.workspace_id = c.workspace_id
JOIN pfas.parties gg ON gg.id = c.grantee_party_id AND gg.workspace_id = c.workspace_id
WHERE c.granter_party_id = $1 AND c.workspace_id = $2
ORDER BY c.granted_at DESC;

-- name: ListConsentsByGrantee :many
SELECT c.*, gt.name AS granter_name, gg.name AS grantee_name
FROM pfas.party_consents c
JOIN pfas.parties gt ON gt.id = c.granter_party_id AND gt.workspace_id = c.workspace_id
JOIN pfas.parties gg ON gg.id = c.grantee_party_id AND gg.workspace_id = c.workspace_id
WHERE c.grantee_party_id = $1 AND c.workspace_id = $2
ORDER BY c.granted_at DESC;

-- name: RevokeConsent :exec
UPDATE pfas.party_consents
SET revoked_at = now()
WHERE id = $1 AND workspace_id = $2 AND revoked_at IS NULL;

-- name: CheckActiveConsent :one
SELECT EXISTS (
    SELECT 1 FROM pfas.party_consents
    WHERE granter_party_id = $1
      AND grantee_party_id = $2
      AND scope = $3
      AND workspace_id = $4
      AND revoked_at IS NULL
      AND (expires_at IS NULL OR expires_at > now())
) AS has_consent;

-- name: CreateFieldParty :exec
INSERT INTO pfas.field_parties (field_id, party_id, workspace_id, association)
VALUES ($1, $2, $3, $4)
ON CONFLICT (field_id, party_id) DO UPDATE SET association = $4;

-- name: ListPartiesByField :many
SELECT p.*, fp.association
FROM pfas.field_parties fp
JOIN pfas.parties p ON p.id = fp.party_id AND p.workspace_id = fp.workspace_id
WHERE fp.field_id = $1 AND fp.workspace_id = $2;

-- name: ListFieldsByParty :many
SELECT f.id, f.facility_id, f.name, f.status
FROM pfas.field_parties fp
JOIN pfas.candidate_fields f ON f.id = fp.field_id AND f.workspace_id = fp.workspace_id
WHERE fp.party_id = $1 AND fp.workspace_id = $2;

-- name: RemoveFieldParty :exec
DELETE FROM pfas.field_parties
WHERE field_id = $1 AND party_id = $2 AND workspace_id = $3;
