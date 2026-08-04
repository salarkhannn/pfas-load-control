# PFAS Load Control

PFAS Load Control is an auditable decision-and-action agent for wastewater treatment
plants and biosolids land-application operators. Module 0 proves the constrained agent
control plane before regulatory decision logic is introduced.

## Module 0

The only available agent is `MIREYE_READINESS`. It executes three fixed, read-only
Mireye calls in order:

1. physical field catalog;
2. plan and credit contract; and
3. authenticated account usage.

The Go API creates each run and its first River job in one PostgreSQL transaction.
Every successful call records its source URL, Mireye request ID, response SHA-256,
retrieval time, duration, HTTP status, and zero credit cost. The browser receives only
sanitized summaries and provenance; it never receives the Mireye token, database URL,
or raw provider response.

Authentication and every credit-consuming or external-write action are deliberately
deferred until their product workflows exist.

## Local development

Requirements: Go 1.26.5, Node 20.20 or newer, and pnpm 10.

1. Copy `.env.example` to `.env` and set `DATABASE_URL` plus either
   `MIREYE_API_TOKEN` or the supported local alias `MIREYE_TOKEN`.
2. Install dependencies with `make install`.
3. Apply migrations with `make migrate`.
4. Start the API and frontend with `make dev`.

The frontend is at `http://localhost:5173`; the API is at `http://localhost:8080`,
with OpenAPI documentation at `/docs`.

Run the complete verification suite with `make verify`.

## Deployment

`render.yaml` defines a free Render Docker web service for Go and a free static site
for React. During initial Blueprint setup, provide:

- `DATABASE_URL`: the Supabase session-pooler URI;
- `MIREYE_API_TOKEN`: the server-only Mireye bearer token;
- `WEB_ORIGIN`: the final static-site origin; and
- `VITE_API_URL`: the final API origin.

The API honors Render's assigned `PORT`. It applies idempotent application and River
migrations on startup, and all durable state remains in Supabase.
