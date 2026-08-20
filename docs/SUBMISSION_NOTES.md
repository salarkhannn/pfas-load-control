# Submission verification

Verified on August 20, 2026 with:

```bash
make verify
```

Result: **pass**.

- `gofmt`: clean across `cmd`, `db`, and `internal`
- Go tests: all packages passed, including judge-demo capacity and mass-balance invariants
- `go vet`: passed
- `staticcheck` 0.7.0: passed
- TypeScript: passed
- ESLint: passed
- Frontend unit tests: 10 passed
- Production build: passed
- Playwright: 15 passed, including the 320px judge-demo replay and desktop judging path

## Public deployment verification

The final judging route is
[`https://fieldproof-inky.vercel.app/judge-demo`](https://fieldproof-inky.vercel.app/judge-demo).
The Vercel frontend builds from `web/` on Node 24 and calls the Northflank API at
`https://p01--fieldproof--6yz627dp2pb8.code.run`. The backend readiness endpoint returned
HTTP 200 and its exact CORS allowlist accepted the final Vercel origin.

A fresh browser profile with no Vercel or Northflank session opened the public route and
completed both agent paths. The unresolved run allocated 28 dry tons to North Forty and
held 24 dry tons while Riverbend East remained under review. The reviewed-evidence action
created a distinct run, reported `SAMPLED_TERRAIN_SCREEN_PASSED`, allocated all 52 dry tons,
and continued to report professional authorization as required. The same flow passed at
desktop width and at 320px with no horizontal overflow. The 320px page issued one initial
create-run request.

Live run `67e58d76-749a-457f-8cfa-25b9303daa23` returned `SUCCEEDED`, calculation status
`READY`, authorization status `REQUIRED`, and zero unallocated dry tons. The downloaded
package SHA-256 was
`d0cb4d9bde5267e866759ec680b972ecc2d0e4fb8af06586fe2a1eea51ac7b0b`, exactly matching
the durable run's decision hash. Repeating a separate public create request with the same
idempotency key returned the same run ID and decision hash.

The judge demo starts a backend `LAND_APPLICATION_READINESS_DECISION` run. All six
decision calls in the unresolved replay execute: the production lab parser, reviewed policy
classifier, captured Mireye batch replay through the real adapter schema and aggregation
rules, operator boundary-input validator, and the placement engine before and after
screening. The trace stores each real input/output, hash, timing, status, and error state.
After those calls are serialized, the canonical freezer emits a separate receipt. The
download contains the six decision calls plus that post-serialization freeze receipt, with
no missing or impossible event inside the frozen payload.

The Mireye fixture is a genuine August 20, 2026 `/v1/fetch/batch` capture with the real
request/response schema, endpoint, request identifier, timestamps, per-field providers,
and exact hashes. It does not claim wetland overlap acreage. Its sampled USGS 3DEP slope
result places Field A into `REVIEW_REQUIRED`; the engine gives it no capacity or allocation.
A separate fixture explicitly identifies the 18.4-acre exclusion as a seeded
`OPERATOR_SUPPLIED` boundary adjustment. That acreage record has no geometry linking it to
the high-slope sample and therefore does not clear the review gate. Tests prove the provider
capture matches the production adapter, every sample preserves its actual provider, acreage
alone cannot resolve slope review, and a confirmed source-linked exception permits the
engine to reconsider the field.

The initial replay allocates 28 dry tons to eligible North Forty and leaves 24 dry tons
unallocated. North Forty is rank 1; review-required Riverbend East is rank 2. The run,
placement plan, interface, database retrieval path, and frozen package preserve those same
statuses, ranks, and allocations.

The **Apply reviewed evidence and rerun** action creates a linked run through a separate
backend endpoint. Its eight decision calls include a revised-boundary Mireye screen and the
full slope-resolution verification. The verifier loads five immutable records: the confirmed
parent boundary, revised-boundary polygon, original Mireye response, revised-boundary Mireye
response, and configured-reviewer authorization artifact. Verification covers record IDs,
byte hashes, evidence type, field, sequential boundary versions, `EPSG:4326`, valid topology,
positive area, containment inside the parent, issuer, reviewer, approval status, timestamps,
expiry, supersession, and bounded provider-screening results.

The accepted subset uses deterministic polygon sampler `POLYGON_STRATIFIED_SCREEN_V1`.
The planner supports valid irregular and concave single-ring GeoJSON polygons, emits
interior and boundary-near locations strictly inside the polygon, and records its version,
75-meter spacing, target and returned counts, and eight-request limit. A target that cannot
be met under the limit, a point outside the polygon, a missing result, changed response, or
sampled slope above 6% grade fails closed. Tests cover irregular and concave polygons and
prove the same geometry produces the same sample coordinates.

Five sampled locations in the seeded case returned slopes below the screening threshold.
Unsampled terrain may contain different conditions. `SAMPLED_TERRAIN_SCREEN_PASSED`
supports calculation and professional review; it does not establish whole-field slope
suitability. The engine derives 31.6 usable acres, calculates 29.92 dry tons of Field A
capacity, retains the 28-dry-ton allocation to rank-1 North Forty, allocates the remaining
24 dry tons to rank-2 Riverbend East, and leaves zero unallocated. Professional
authorization remains required. The interface and frozen package preserve the same parent
run, evidence records, verification result, ranks, and allocation.

Accepted issuer and reviewer roles come from versioned program policy. The seeded case
uses `DEMO_FIELD_BOUNDARY_ISSUER` and `DEMO_PLACEMENT_REVIEWER`; these are demonstration
roles and do not represent universal EGLE requirements. An unconfigured role fails, while
a separately configured issuer and reviewer succeed only when the immutable evidence,
scope, timing, expiration, and supersession checks also pass.

Tests fail closed for unknown evidence and authorization IDs; changed boundary, parent,
source, revised-screen, or authorization hashes; unsupported types; another field; stale
boundaries; missing, unauthorized, or superseded approvals; expired records; invalid or
empty polygons; off-site or out-of-parent polygons; inadequate retained-land sampling; a
retained excessive-slope sample; and a revised polygon that still contains the original
high-slope sample. Exact conversion tests prove that a slope at 6% grade is not rounded into
a block while the next representable value above it is blocked. Reusing an idempotency key
for either action returns the original frozen run. A new action key creates one new run.

Migration `00020` permits the reviewed run's calculation status (`READY`) in the durable
judge-demo table. The API and package separately report `runStatus: SUCCEEDED`,
`calculationStatus`, `authorizationStatus: REQUIRED`, and `authorizationRequired: true`;
no successful calculation is represented as authorization to apply. A live PostgreSQL check
created the unresolved run, created one linked
reviewed run, returned the same reviewed run for the same idempotency key, and retrieved it
after an API restart. The downloaded package SHA-256 remained identical to the stored and
displayed decision hash.

Runs are persisted in PostgreSQL by migration `00018`; migration `00019` adds a dedicated
`BYTEA` column for the exact frozen package artifact. Both survive an API restart.
`Idempotency-Key`, a cross-process PostgreSQL advisory lock, and a database uniqueness
constraint prevent repeated create requests from executing tools or producing a second
run. The React Strict Mode loader emits one initial request. An explicit **Rerun agent**
action uses a fresh key and produces a fresh run ID.

The exact canonical decision-payload bytes and payload hash are placed in an outer package
envelope with a `freezeReceipt`. The receipt records the subsequent freeze event, artifact
ID, payload hash, and completion time. The exact envelope bytes are stored with the run;
every retrieval recomputes the envelope hash and rejects a mismatch. A tamper test modifies
the stored bytes and confirms an integrity failure. The verified artifact is downloadable
from `/api/v1/judge-demo/runs/{id}/package`.

Practitioner evidence rejects the original PFAS-automation thesis for several programs
and identifies farmer-plant discovery and communication as a stronger lead. The current
buyer hypothesis is a third-party land-application contractor or a utility that actively
manages land application. Labor, budget, pricing, and willingness to pay remain
unvalidated and are not represented as complete.

Six additional outreach messages were reported sent on August 20, 2026 and are recorded as
**Response pending** in `docs/CUSTOMER_EVIDENCE.md`. Their exact redacted threads and roles
were not supplied to the repository, so the notes do not invent or quote them. Pending
outreach provides no validation.
