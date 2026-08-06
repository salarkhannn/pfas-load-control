-- +goose Up
ALTER TABLE pfas.field_geometry_versions
    DROP CONSTRAINT field_geometry_versions_source_check;

ALTER TABLE pfas.field_geometry_versions
    ALTER COLUMN confirmed_at DROP DEFAULT,
    ALTER COLUMN confirmed_at DROP NOT NULL;

UPDATE pfas.field_geometry_versions
SET source = CASE source
    WHEN 'OPERATOR_GEOJSON' THEN 'UPLOADED_GEOJSON'
    WHEN 'MIREYE_PARCEL_CONFIRMED' THEN 'MIREYE_PARCEL'
END,
confirmed_at = CASE
    WHEN source = 'OPERATOR_GEOJSON' THEN NULL
    ELSE confirmed_at
END;

ALTER TABLE pfas.field_geometry_versions
    ADD CONSTRAINT field_geometry_versions_source_check
    CHECK (source IN ('UPLOADED_GEOJSON', 'MIREYE_PARCEL'));

-- +goose Down
ALTER TABLE pfas.field_geometry_versions
    DROP CONSTRAINT field_geometry_versions_source_check;

UPDATE pfas.field_geometry_versions
SET source = CASE source
    WHEN 'UPLOADED_GEOJSON' THEN 'OPERATOR_GEOJSON'
    WHEN 'MIREYE_PARCEL' THEN 'MIREYE_PARCEL_CONFIRMED'
END,
confirmed_at = COALESCE(confirmed_at, now());

ALTER TABLE pfas.field_geometry_versions
    ALTER COLUMN confirmed_at SET DEFAULT now(),
    ALTER COLUMN confirmed_at SET NOT NULL;

ALTER TABLE pfas.field_geometry_versions
    ADD CONSTRAINT field_geometry_versions_source_check
    CHECK (source IN ('OPERATOR_GEOJSON', 'MIREYE_PARCEL_CONFIRMED'));
