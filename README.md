# FieldProof

FieldProof is a persisted land-application evidence and placement agent. It combines
Mireye terrain evidence with laboratory reports, field boundaries, operating records,
and application history. It identifies missing evidence, proposes a conservative
placement plan, and prepares a cited record for professional authorization. A PFAS
laboratory result and Michigan classification are batch inputs; they are not the
product's sole purpose and do not establish field readiness. The repository and Go
module retain their original `pfas-load-control` technical names.

The primary buyer hypothesis is a third-party land-application contractor or a utility
that actively manages its own land-application program. Practitioner correspondence
rejected several parts of the original PFAS-automation thesis and identified
farmer-plant discovery and communication as a stronger problem lead. Repeated labor,
budget ownership, pricing, and willingness to pay remain unvalidated. See
[`docs/CUSTOMER_EVIDENCE.md`](docs/CUSTOMER_EVIDENCE.md). The next contractor interview is
prepared in [`docs/CONTRACTOR_INTERVIEW.md`](docs/CONTRACTOR_INTERVIEW.md); no response or
purchase interest is claimed.

### Buyer hypothesis

| Question | Current hypothesis |
| --- | --- |
| Economic buyer | Contractor owner, operations manager, or utility biosolids program manager |
| Daily user | Land-application coordinator, operator, or configured program reviewer |
| Current alternative | Spreadsheets, GIS, email, paper agreements, consultant review, and MiEnviro |
| Buying trigger | Field shortages, expiring records, new participating farms, repeated evidence follow-up, or a failed application review |
| Pilot | Reconstruct 20 historical placement decisions without influencing live applications |
| Success measures | Missing evidence detected, reviewer agreement, preparation time, package completeness, and false-clear rate |

Labor cost, budget ownership, pricing, pilot interest, and willingness to pay remain
unvalidated. Unanswered outreach is activity, not demand evidence.

## Decision agent

```text
Candidate fields + farmer and operator records
        │ finds fields; checks agreements, soil tests, crop, acreage, and loading
        ▼
Qualified operating facts ───── missing or uncertain ──► WAITING_FOR_INPUT
        │
        ▼
Batch evidence, including PFAS report + Michigan classification
        │ treats policy as one bounded input; selects field-investigation tools
        ▼
Mireye physical evidence + RMP, crop, approval, and loading records
        │ detects gaps and constraints
        ├── critical evidence missing ──────────────────► REVIEW_REQUIRED
        │
        ▼
Conservative placement-capacity calculation
        ├── prohibited batch ───────────────────────────► ALTERNATIVE_MANAGEMENT
        ├── insufficient capacity ──────────────────────► PARTIALLY_ALLOCATED
        └── supported proposal ─────────────────────────► PROPOSED_PLACEMENT
                                                              │
                                                              ▼
Frozen cited package + controlled handoff
                                                              │
                                                              ▼
                                            PROFESSIONAL_AUTHORIZATION_REQUIRED
```

The software gathers and qualifies evidence, proposes an allocation, and prepares a
handoff. The responsible contractor or active-program utility and its agronomic
professional authorize any real application.

For a deterministic walkthrough, choose **Open judge demo** on the landing page. The
prepared case is visibly labeled as seeded, shows a captured Mireye slope finding change
the ranking between two fields, links each decisive fact to its operational effect, and
can be replayed without live customer data. `POST /api/v1/judge-demo/runs` executes the
same lab parser, reviewed policy classifier, physical-evidence aggregation, placement
engine, and canonical package freezer used by the product. Frozen adapters replace
only unstable or credit-consuming provider calls. Each tool record is emitted by the
execution wrapper with its actual serialized input and output, hashes, timing, status,
and error state.

The Mireye fixture is a genuine `/v1/fetch/batch` capture from August 20, 2026. It keeps
the real request and response schemas, endpoint, request identifier, timestamps, source
datasets, and exact request/response hashes. The captured USGS 3DEP facts include a
9.425-degree sampled maximum slope. The engine converts that value to 16.6% grade using
`tan(degrees × π / 180) × 100`; Michigan's 6% grade threshold is 3.43 degrees. The
placement engine therefore marks Field A
`REVIEW_REQUIRED` and does not calculate capacity or allocate material there.

Mireye did **not** return excluded acreage. A separate seeded operator record supplies
the boundary adjustment and states its provenance explicitly. The replay validates the
operator arithmetic before the placement engine runs:

```text
Before screening: Field A capacity = 50 acres × 1.2 − 8 = 52.00 dry tons
Operator input:  50 recorded acres − 18.4 excluded acres = 31.6 effective acres
After screening: Field A = REVIEW_REQUIRED; capacity not calculated; allocation 0
Current plan:    Field B 28.00 + unallocated 24.00 = batch 52.00
```

The acreage record has no geometry proving that its exclusion removed the high-slope
sample, so it cannot clear the slope gate. The engine can reconsider Field A only after it
loads an immutable evidence record, verifies its hash and authorization, and proves from
the stored polygon that each captured high-slope sample falls outside the revised boundary. Free-form
references, acreage alone, and invented approval text fail closed.

The demo's **Apply reviewed evidence and rerun** action creates a second persisted run linked
to the unresolved run. It loads a seeded revised-boundary artifact, its immutable parent
boundary, the original Mireye source artifact, a fresh revised-boundary Mireye screening,
and a separate reviewer-authorization artifact. The verifier checks that both polygons use
`EPSG:4326`, the revised version immediately follows the parent version, the parent bytes
match its hash, the revised geometry has valid topology and positive area, and the revised
polygon lies wholly inside or on the confirmed parent boundary. It rejects off-site,
out-of-boundary, empty, invalid, stale, disconnected, or unrelated geometry.

The revised boundary is screened with versioned deterministic polygon sampler
`POLYGON_STRATIFIED_SCREEN_V1`. It supports valid irregular and concave single-ring
GeoJSON polygons, places boundary-near and interior locations strictly inside the polygon,
and records its algorithm version, 75-meter minimum spacing, target and returned counts,
and eight-request credit cap. The same geometry produces the same locations. If the
configured target cannot be met within the request limit, a result is missing, a returned
location falls outside the polygon, the response hash changes, or a sampled slope exceeds
six-percent grade, the field returns `REVIEW_REQUIRED`.

The frozen capture stores the real Mireye `/v1/fetch/batch` request and response, provider
sources, request ID, retrieval time, hashes, planner metadata, and five returned USGS 3DEP
slope values. In this demonstration, five sampled locations returned slopes below the
screening threshold. Unsampled terrain may contain different conditions. The status
`SAMPLED_TERRAIN_SCREEN_PASSED` supports calculation and professional review; it does not
establish whole-field slope suitability. The demonstration record derives 31.6 usable
acres from its parent-bound polygon, so the placement engine calculates:

```text
Verified Field A capacity = 31.6 acres × 1.2 − 8 = 29.92 dry tons
Reviewed plan: Field B 28.00 + Field A 24.00 + unallocated 0 = batch 52.00
```

Issuer and reviewer roles come from versioned program policy rather than hard-coded
professional titles. The seeded policy uses `DEMO_FIELD_BOUNDARY_ISSUER` and
`DEMO_PLACEMENT_REVIEWER`; these are configured demonstration roles, not universal EGLE
requirements. The verifier requires an authorized issuer, a separately configured
reviewer, matching immutable evidence, approval after evidence capture, and an unexpired,
unsuperseded authorization. The reviewer authorization remains a demonstration artifact,
not an EGLE approval or permission to apply. Both API modes separate `runStatus`, `calculationStatus`, and
`authorizationStatus`; even a reviewed calculation returns `authorizationRequired: true`.
The interface therefore says **Run completed**, **Calculation ready**, and **Professional
authorization required**, never that the application itself is ready.

The second run records `mireye.revised-boundary.screen` and `slope-resolution.verify` as
executed tool calls and freezes the parent boundary, revised geometry, original and revised
Mireye artifacts, authorization record, slope conversion, updated field status, and
allocation in its package. The judge can switch between both frozen runs in the page.

Judge-demo runs are stored in PostgreSQL, including the fixture version, executed tool
records, plans, citations, source hashes, review question, exact canonical package bytes,
package hash, and timestamps. The canonical decision payload contains only calls completed
before serialization. A separate outer `freezeReceipt` identifies the subsequent
`decisionpackage.freeze` action, payload hash, artifact ID, and completion time; the package
envelope has its own hash. Retrieval recomputes the envelope hash from the stored bytes and
rejects a modified artifact. The verified JSON is downloadable from the demo.
`GET /api/v1/judge-demo/runs/{id}` therefore returns the same package after an API
restart. The create endpoint requires an `Idempotency-Key`; a PostgreSQL advisory lock
serializes the check-and-execute boundary across API instances, and the unique database
key returns the stored run on repeated delivery. An explicit rerun sends a fresh key.
The React Strict Mode loader also shares one initial request promise so one page visit
starts one run.

## Mireye setup guard

Before a live investigation, the `MIREYE_READINESS` setup guard executes three fixed,
read-only Mireye calls in order:

1. physical field catalog;
2. plan and credit contract; and
3. authenticated account usage.

The Go API creates each run and its first River job in one PostgreSQL transaction.
Every successful call records its source URL, Mireye request ID, response SHA-256,
retrieval time, duration, HTTP status, and zero credit cost. The browser receives only
sanitized summaries and provenance; it never receives the Mireye token, database URL,
or raw provider response.

The setup guard does not make batch or field decisions. Authentication and every
credit-consuming or external-write action remain bounded by their product workflows.

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

In the interface, that internal `READY` field state is described as **Facts complete**.
It means the minimum operating facts are present for subsequent screening; it does not
mean that the field is agronomically suitable or that an application is authorized.

## Modules 9–12

The coordination workspace records parties, candidate-field coordination, registry
references, and application history. Assignment and confirmation are separate actions.
Confirmations must occur in farmer, contractor, then plant order, and the resulting state
is **Coordination complete**, not “ready to apply.” Coordination changes are committed
transactionally and a rejection requires a recorded reason.

The current browser workspace key isolates prototype data but does not authenticate a
named person. Coordination confirmations and action-review records are therefore suitable
for competition demonstration only. Production use requires authenticated accounts,
role-based authorization, and signed actor attribution. No software state in this project
replaces professional agronomic judgment, direct farm knowledge, or regulatory approval.

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
The latest full verification result is recorded in `docs/SUBMISSION_NOTES.md`.

## Deployment

`render.yaml` defines a free Render Docker web service for Go and a free static site for
React. The API can spin down after 15 minutes without inbound traffic, so open the public
site shortly before judging to absorb the cold start. During initial Blueprint setup,
provide:

- `DATABASE_URL`: the Supabase session-pooler URI;
- `MIREYE_API_TOKEN`: the server-only Mireye bearer token;
- `WEB_ORIGIN`: the final static-site origin; and
- `VITE_API_URL`: the final API origin.

The API honors Render's assigned `PORT`. It applies idempotent application and River
migrations on startup, and all durable state remains in Supabase.
