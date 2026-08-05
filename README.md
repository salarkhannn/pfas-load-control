# PFAS Load Control

PFAS Load Control is an auditable decision-and-action agent for wastewater treatment
plants and biosolids land-application operators. The implemented flow proves the
constrained agent control plane, turns laboratory reports into confirmed evidence,
applies versioned Michigan PFAS policy, and builds a geometry-backed candidate-field
inventory with live Mireye location evidence.

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

## Module 1

Operators can attach a PDF, CSV, or JSON PFAS report to a facility and biosolids batch.
A bounded background job extracts PFOS and PFOA results, units, weight basis,
qualifiers, detection limits, report metadata, and the exact source page or row.
Machine-readable PDFs use their text layer; scanned pages use OCR. The review screen
keeps the original report beside the extracted evidence, requires missing or
conflicting values to be corrected, and records an immutable confirmed version.

Original files and evidence are scoped to a browser-generated workspace capability;
the API exposes neither report listings nor public file URLs. Account authentication
remains deferred as planned.

## Module 2

Confirmed PFOS and PFOA evidence is evaluated with exact decimal arithmetic against
the active, reviewed Michigan rule pack. The immutable decision records the matched
rule, required actions, citations, rule-pack checksum, and input hash. Policy
classification never implies that a candidate field is approved.

## Module 3

Operators can add candidate fields by address, coordinates, APN plus county, GeoJSON,
or CSV. Mireye resolves human locators and returns cited parcel-match evidence;
ambiguous results require an explicit choice. A parcel becomes the controlling field
boundary only after the operator confirms it, while an uploaded field polygon is
preserved directly. Every boundary is validated and versioned in PostGIS.

The field ledger captures RMP approval, MiEnviro site ID, usable acres, agronomic
rate, prior loading, intended use, known nearby constraints, and access constraints.
Missing required facts remain visible named gaps, and a field becomes ready only when
its actual geometry and required operating facts are confirmed.

## Local development

Requirements: Go 1.26.5, Node 20.20 or newer, and pnpm 10.

1. Copy `.env.example` to `.env` and set `DATABASE_URL` plus either
   `MIREYE_API_TOKEN` or the supported local alias `MIREYE_TOKEN`.
2. Install dependencies with `make install`.
3. Apply migrations with `make migrate`.
4. Start the API and frontend with `make dev`.

The frontend is at `http://localhost:5174`; the API is at `http://localhost:8080`,
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
