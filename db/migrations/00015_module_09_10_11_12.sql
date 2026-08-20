-- +goose Up

-- Module 9: Party Identity + Consent

CREATE TYPE pfas.party_role AS ENUM ('PLANT', 'CONTRACTOR', 'FARMER');

CREATE TABLE pfas.parties (
    id UUID PRIMARY KEY,
    workspace_id UUID NOT NULL REFERENCES pfas.workspaces(id) ON DELETE CASCADE,
    role pfas.party_role NOT NULL,
    name TEXT NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 200),
    email TEXT NOT NULL CHECK (length(btrim(email)) BETWEEN 3 AND 320),
    phone TEXT NOT NULL DEFAULT '',
    latitude DOUBLE PRECISION,
    longitude DOUBLE PRECISION,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, email),
    UNIQUE (id, workspace_id)
);

CREATE INDEX parties_workspace_idx ON pfas.parties(workspace_id, role);

CREATE TABLE pfas.party_consents (
    id UUID PRIMARY KEY,
    workspace_id UUID NOT NULL REFERENCES pfas.workspaces(id) ON DELETE CASCADE,
    granter_party_id UUID NOT NULL,
    grantee_party_id UUID NOT NULL,
    scope TEXT NOT NULL CHECK (length(btrim(scope)) BETWEEN 1 AND 200),
    purpose TEXT NOT NULL CHECK (length(btrim(purpose)) BETWEEN 1 AND 500),
    granted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    FOREIGN KEY (granter_party_id, workspace_id) REFERENCES pfas.parties(id, workspace_id) ON DELETE RESTRICT,
    FOREIGN KEY (grantee_party_id, workspace_id) REFERENCES pfas.parties(id, workspace_id) ON DELETE RESTRICT,
    UNIQUE (id, workspace_id)
);

CREATE INDEX consents_granter_idx ON pfas.party_consents(granter_party_id, revoked_at);
CREATE INDEX consents_grantee_idx ON pfas.party_consents(grantee_party_id, revoked_at);

-- Module 9: Party-Field associations

CREATE TABLE pfas.field_parties (
    field_id UUID NOT NULL,
    party_id UUID NOT NULL,
    workspace_id UUID NOT NULL REFERENCES pfas.workspaces(id) ON DELETE CASCADE,
    association TEXT NOT NULL CHECK (association IN ('OWNER', 'APPLICANT', 'CONTRACTOR')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (field_id, workspace_id) REFERENCES pfas.candidate_fields(id, workspace_id) ON DELETE CASCADE,
    FOREIGN KEY (party_id, workspace_id) REFERENCES pfas.parties(id, workspace_id) ON DELETE RESTRICT,
    PRIMARY KEY (field_id, party_id),
    UNIQUE (field_id, party_id, workspace_id)
);

CREATE INDEX field_parties_party_idx ON pfas.field_parties(party_id, workspace_id);

-- Module 10: Discovery Registry

CREATE TABLE pfas.registry_entries (
    id UUID PRIMARY KEY,
    workspace_id UUID NOT NULL REFERENCES pfas.workspaces(id) ON DELETE CASCADE,
    party_id UUID,
    entry_type TEXT NOT NULL CHECK (entry_type IN ('PLANT', 'FIELD', 'CONTRACTOR')),
    name TEXT NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 300),
    data JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(data) = 'object'),
    latitude DOUBLE PRECISION,
    longitude DOUBLE PRECISION,
    search_vector TSVECTOR,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (party_id, workspace_id) REFERENCES pfas.parties(id, workspace_id) ON DELETE SET NULL,
    UNIQUE (id, workspace_id)
);

CREATE INDEX registry_entries_type_idx ON pfas.registry_entries(entry_type, workspace_id);
CREATE INDEX registry_entries_search_idx ON pfas.registry_entries USING GIN(search_vector);

CREATE TABLE pfas.registry_matches (
    id UUID PRIMARY KEY,
    workspace_id UUID NOT NULL REFERENCES pfas.workspaces(id) ON DELETE CASCADE,
    plant_entry_id UUID NOT NULL,
    field_entry_id UUID NOT NULL,
    score DOUBLE PRECISION NOT NULL CHECK (score >= 0 AND score <= 1),
    reasons JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(reasons) = 'array'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (plant_entry_id, workspace_id) REFERENCES pfas.registry_entries(id, workspace_id) ON DELETE CASCADE,
    FOREIGN KEY (field_entry_id, workspace_id) REFERENCES pfas.registry_entries(id, workspace_id) ON DELETE CASCADE,
    UNIQUE (id, workspace_id)
);

CREATE INDEX registry_matches_plant_idx ON pfas.registry_matches(plant_entry_id, score DESC);

-- Module 11: Field Readiness Coordination

CREATE TYPE pfas.coordination_status AS ENUM (
    'NOT_STARTED', 'FARMER_CONFIRMED', 'CONTRACTOR_CONFIRMED',
    'PLANT_CONFIRMED', 'READY', 'REJECTED'
);

CREATE TABLE pfas.coordination_workflows (
    id UUID PRIMARY KEY,
    workspace_id UUID NOT NULL REFERENCES pfas.workspaces(id) ON DELETE CASCADE,
    batch_id UUID,
    field_id UUID,
    status pfas.coordination_status NOT NULL DEFAULT 'NOT_STARTED',
    created_by_party_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (created_by_party_id, workspace_id) REFERENCES pfas.parties(id, workspace_id) ON DELETE RESTRICT,
    FOREIGN KEY (field_id, workspace_id) REFERENCES pfas.candidate_fields(id, workspace_id) ON DELETE SET NULL,
    UNIQUE (id, workspace_id)
);

CREATE INDEX coordination_workflows_field_idx ON pfas.coordination_workflows(field_id, status);
CREATE INDEX coordination_workflows_status_idx ON pfas.coordination_workflows(status, workspace_id);

CREATE TYPE pfas.coordination_step_role AS ENUM ('FARMER', 'CONTRACTOR', 'PLANT');

CREATE TABLE pfas.coordination_steps (
    id UUID PRIMARY KEY,
    workspace_id UUID NOT NULL REFERENCES pfas.workspaces(id) ON DELETE CASCADE,
    workflow_id UUID NOT NULL,
    party_id UUID,
    step_role pfas.coordination_step_role NOT NULL,
    step_type TEXT NOT NULL CHECK (length(btrim(step_type)) BETWEEN 1 AND 100),
    status TEXT NOT NULL CHECK (status IN ('PENDING', 'CONFIRMED', 'REJECTED')),
    notes TEXT NOT NULL DEFAULT '',
    confirmed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (workflow_id, workspace_id) REFERENCES pfas.coordination_workflows(id, workspace_id) ON DELETE CASCADE,
    FOREIGN KEY (party_id, workspace_id) REFERENCES pfas.parties(id, workspace_id) ON DELETE SET NULL,
    UNIQUE (id, workspace_id)
);

CREATE INDEX coordination_steps_workflow_idx ON pfas.coordination_steps(workflow_id, step_role);

CREATE TABLE pfas.coordination_documents (
    id UUID PRIMARY KEY,
    workspace_id UUID NOT NULL REFERENCES pfas.workspaces(id) ON DELETE CASCADE,
    workflow_id UUID NOT NULL,
    party_id UUID NOT NULL,
    doc_type TEXT NOT NULL CHECK (length(btrim(doc_type)) BETWEEN 1 AND 100),
    filename TEXT NOT NULL CHECK (length(btrim(filename)) BETWEEN 1 AND 500),
    file_hash TEXT NOT NULL CHECK (length(file_hash) = 64),
    mime_type TEXT NOT NULL DEFAULT 'application/octet-stream',
    size_bytes BIGINT NOT NULL DEFAULT 0 CHECK (size_bytes >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (workflow_id, workspace_id) REFERENCES pfas.coordination_workflows(id, workspace_id) ON DELETE CASCADE,
    FOREIGN KEY (party_id, workspace_id) REFERENCES pfas.parties(id, workspace_id) ON DELETE RESTRICT,
    UNIQUE (id, workspace_id)
);

CREATE INDEX coordination_documents_workflow_idx ON pfas.coordination_documents(workflow_id);

CREATE TABLE pfas.coordination_notifications (
    id UUID PRIMARY KEY,
    workspace_id UUID NOT NULL REFERENCES pfas.workspaces(id) ON DELETE CASCADE,
    workflow_id UUID NOT NULL,
    recipient_party_id UUID NOT NULL,
    event_type TEXT NOT NULL CHECK (length(btrim(event_type)) BETWEEN 1 AND 100),
    message TEXT NOT NULL DEFAULT '',
    read_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (workflow_id, workspace_id) REFERENCES pfas.coordination_workflows(id, workspace_id) ON DELETE CASCADE,
    FOREIGN KEY (recipient_party_id, workspace_id) REFERENCES pfas.parties(id, workspace_id) ON DELETE RESTRICT,
    UNIQUE (id, workspace_id)
);

CREATE INDEX coordination_notifications_recipient_idx ON pfas.coordination_notifications(recipient_party_id, read_at);

-- Module 12: Application Tracking

CREATE TABLE pfas.application_records (
    id UUID PRIMARY KEY,
    workspace_id UUID NOT NULL REFERENCES pfas.workspaces(id) ON DELETE CASCADE,
    batch_id UUID,
    field_id UUID NOT NULL,
    contractor_party_id UUID NOT NULL,
    application_date DATE NOT NULL,
    dry_tons NUMERIC(12, 4) NOT NULL CHECK (dry_tons > 0),
    rate_dry_tons_per_acre NUMERIC(8, 4) NOT NULL CHECK (rate_dry_tons_per_acre > 0),
    acres_applied NUMERIC(10, 4) NOT NULL CHECK (acres_applied > 0),
    weather_conditions TEXT NOT NULL DEFAULT '',
    field_condition_notes TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (field_id, workspace_id) REFERENCES pfas.candidate_fields(id, workspace_id) ON DELETE RESTRICT,
    FOREIGN KEY (contractor_party_id, workspace_id) REFERENCES pfas.parties(id, workspace_id) ON DELETE RESTRICT,
    UNIQUE (id, workspace_id)
);

CREATE INDEX application_records_field_idx ON pfas.application_records(field_id, application_date);
CREATE INDEX application_records_contractor_idx ON pfas.application_records(contractor_party_id, application_date);

CREATE TABLE pfas.field_loading_ledger (
    id UUID PRIMARY KEY,
    workspace_id UUID NOT NULL REFERENCES pfas.workspaces(id) ON DELETE CASCADE,
    field_id UUID NOT NULL,
    year INTEGER NOT NULL CHECK (year >= 2020 AND year <= 2100),
    cumulative_dry_tons NUMERIC(12, 4) NOT NULL DEFAULT 0 CHECK (cumulative_dry_tons >= 0),
    last_application_date DATE,
    last_updated TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (field_id, workspace_id) REFERENCES pfas.candidate_fields(id, workspace_id) ON DELETE CASCADE,
    UNIQUE (field_id, year, workspace_id),
    UNIQUE (id, workspace_id)
);

CREATE INDEX loading_ledger_field_idx ON pfas.field_loading_ledger(field_id, year);

CREATE TABLE pfas.application_confirmations (
    id UUID PRIMARY KEY,
    workspace_id UUID NOT NULL REFERENCES pfas.workspaces(id) ON DELETE CASCADE,
    application_id UUID NOT NULL,
    farmer_party_id UUID NOT NULL,
    confirmed BOOLEAN NOT NULL DEFAULT false,
    notes TEXT NOT NULL DEFAULT '',
    confirmed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (application_id, workspace_id) REFERENCES pfas.application_records(id, workspace_id) ON DELETE CASCADE,
    FOREIGN KEY (farmer_party_id, workspace_id) REFERENCES pfas.parties(id, workspace_id) ON DELETE RESTRICT,
    UNIQUE (id, workspace_id)
);

CREATE INDEX application_confirmations_app_idx ON pfas.application_confirmations(application_id);

-- +goose Down
DROP TABLE IF EXISTS pfas.application_confirmations;
DROP TABLE IF EXISTS pfas.field_loading_ledger;
DROP TABLE IF EXISTS pfas.application_records;
DROP TABLE IF EXISTS pfas.coordination_notifications;
DROP TABLE IF EXISTS pfas.coordination_documents;
DROP TABLE IF EXISTS pfas.coordination_steps;
DROP TABLE IF EXISTS pfas.coordination_workflows;
DROP TYPE IF EXISTS pfas.coordination_step_role;
DROP TYPE IF EXISTS pfas.coordination_status;
DROP TABLE IF EXISTS pfas.registry_matches;
DROP TABLE IF EXISTS pfas.registry_entries;
DROP TABLE IF EXISTS pfas.field_parties;
DROP TABLE IF EXISTS pfas.party_consents;
DROP TABLE IF EXISTS pfas.parties;
DROP TYPE IF EXISTS pfas.party_role;
