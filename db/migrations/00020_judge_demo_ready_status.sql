-- +goose Up

ALTER TABLE pfas.judge_demo_runs
    DROP CONSTRAINT IF EXISTS judge_demo_runs_status_check;

ALTER TABLE pfas.judge_demo_runs
    ADD CONSTRAINT judge_demo_runs_status_check
    CHECK (status IN ('REVIEW_REQUIRED', 'READY', 'SUCCEEDED', 'FAILED'));

-- +goose Down

ALTER TABLE pfas.judge_demo_runs
    DROP CONSTRAINT IF EXISTS judge_demo_runs_status_check;

ALTER TABLE pfas.judge_demo_runs
    ADD CONSTRAINT judge_demo_runs_status_check
    CHECK (status IN ('REVIEW_REQUIRED', 'SUCCEEDED', 'FAILED'));
