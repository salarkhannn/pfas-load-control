-- +goose Up
INSERT INTO pfas.field_data_gaps (id, field_id, code, detail, resolution, status)
SELECT gen_random_uuid(), field.id, 'GEOMETRY_UNCONFIRMED',
       'The actual application boundary is not confirmed.',
       'Compare the uploaded outline with the approved site record and confirm the match.',
       'OPEN'
FROM pfas.candidate_fields AS field
JOIN pfas.field_geometry_versions AS geometry
  ON geometry.id = field.current_geometry_id AND geometry.field_id = field.id
WHERE geometry.confirmed_at IS NULL
ON CONFLICT (field_id, code) DO UPDATE
SET detail = EXCLUDED.detail,
    resolution = EXCLUDED.resolution,
    status = 'OPEN',
    resolved_at = NULL;

UPDATE pfas.candidate_fields AS field
SET status = 'NEEDS_GEOMETRY', updated_at = now()
FROM pfas.field_geometry_versions AS geometry
WHERE geometry.id = field.current_geometry_id
  AND geometry.field_id = field.id
  AND geometry.confirmed_at IS NULL;

-- +goose Down
UPDATE pfas.field_data_gaps AS gap
SET status = 'RESOLVED', resolved_at = now()
FROM pfas.candidate_fields AS field
JOIN pfas.field_geometry_versions AS geometry
  ON geometry.id = field.current_geometry_id AND geometry.field_id = field.id
WHERE gap.field_id = field.id
  AND gap.code = 'GEOMETRY_UNCONFIRMED'
  AND geometry.source = 'UPLOADED_GEOJSON';

UPDATE pfas.candidate_fields AS field
SET status = CASE
        WHEN field.rmp_approved = true
         AND field.usable_acres IS NOT NULL
         AND field.agronomic_rate_dry_tons_acre IS NOT NULL
         AND field.prior_loading_dry_tons IS NOT NULL
        THEN 'READY'
        ELSE 'NEEDS_DETAILS'
    END,
    updated_at = now()
FROM pfas.field_geometry_versions AS geometry
WHERE geometry.id = field.current_geometry_id
  AND geometry.field_id = field.id
  AND geometry.source = 'UPLOADED_GEOJSON';
