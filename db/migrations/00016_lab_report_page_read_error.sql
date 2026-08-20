-- +goose Up

-- Track pages that could not be read (e.g. scanned pages when OCR is unavailable).
ALTER TABLE pfas.lab_report_pages
    ADD COLUMN read_error TEXT;

-- +goose Down

ALTER TABLE pfas.lab_report_pages
    DROP COLUMN read_error;