-- +goose Up

ALTER TABLE pfas.judge_demo_runs ADD COLUMN package_artifact BYTEA;

UPDATE pfas.judge_demo_runs
SET package_artifact = decode(record #>> '{package,artifact}', 'base64')
WHERE record #>> '{package,artifact}' IS NOT NULL;

UPDATE pfas.judge_demo_runs SET package_artifact = '\x' WHERE package_artifact IS NULL;
ALTER TABLE pfas.judge_demo_runs ALTER COLUMN package_artifact SET NOT NULL;

-- +goose Down

ALTER TABLE pfas.judge_demo_runs DROP COLUMN IF EXISTS package_artifact;
