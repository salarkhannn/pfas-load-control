# Submission notes

Last verified: August 20, 2026

## Public deployment

- Application: [fieldproof-inky.vercel.app/judge-demo](https://fieldproof-inky.vercel.app/judge-demo)
- API health: [p01--fieldproof--6yz627dp2pb8.code.run/health/ready](https://p01--fieldproof--6yz627dp2pb8.code.run/health/ready)
- Frontend: Vercel, Node 24
- Backend and PostgreSQL: Northflank

I tested the public route in a fresh browser at desktop width and at 320px. Both agent
paths completed without horizontal overflow. The page made one initial request, and each
rerun made one new request.

## Verification

I ran:

```bash
make verify
```

The command passed Go formatting, tests, vet, static analysis, TypeScript, ESLint, 10
frontend unit tests, the production build, and all 15 Playwright tests.

## Prepared judging path

The first run uses frozen demonstration evidence and the production service interfaces.
Mireye finds a high-slope sample on Riverbend East. The placement engine then:

- allocates 28 dry tons to North Forty;
- places Riverbend East under review; and
- holds the remaining 24 dry tons.

The reviewed-evidence action creates a separate persisted run. It verifies the revised
boundary's lineage, immutable evidence hashes, configured demonstration roles, and a new
Mireye screen before recalculating. Five sampled locations return slopes below the
screening threshold, so the engine can calculate placement for the held material.

This is a sampled terrain screen, not proof of whole-field suitability. Unsampled terrain
may differ, and professional authorization is still required.

## Audit and safety checks

- Each displayed decision call comes from an executed service call.
- Tool inputs, outputs, timings, status, sources, and hashes are recorded.
- The exact canonical decision payload is stored and downloadable.
- The freeze receipt is recorded outside the payload because freezing happens after the
  decision calls.
- Package hashes are recomputed on retrieval, and altered packages fail verification.
- Runs and packages survive API restarts.
- Idempotency keys prevent duplicate runs.
- Calculation readiness and professional authorization are separate API and UI states.
- All seeded records are labeled as demonstration evidence.

## Commercial status

The original PFAS-automation idea was not supported by practitioner feedback. FieldProof
now treats PFAS as one input in a broader land-application readiness workflow. The buyer
hypothesis is a third-party land-application contractor or a utility that manages its own
land-application program.

Labor cost, budget ownership, pricing, pilot interest, and willingness to pay remain
unvalidated. See [CUSTOMER_EVIDENCE.md](CUSTOMER_EVIDENCE.md).
