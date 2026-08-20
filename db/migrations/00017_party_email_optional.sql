-- +goose Up

-- Allow parties without an email: empty string means "no email on file".
ALTER TABLE pfas.parties
    DROP CONSTRAINT parties_workspace_id_email_key,
    DROP CONSTRAINT parties_email_check,
    ADD CONSTRAINT parties_email_check CHECK (btrim(email) = '' OR length(btrim(email)) BETWEEN 3 AND 320);

CREATE UNIQUE INDEX parties_workspace_id_email_uniq
    ON pfas.parties (workspace_id, email)
    WHERE btrim(email) <> '';

-- +goose Down

DROP INDEX IF EXISTS parties_workspace_id_email_uniq;
ALTER TABLE pfas.parties
    DROP CONSTRAINT parties_email_check,
    ADD CONSTRAINT parties_email_check CHECK (length(btrim(email)) BETWEEN 3 AND 320);
ALTER TABLE pfas.parties ADD CONSTRAINT parties_workspace_id_email_key UNIQUE (workspace_id, email);