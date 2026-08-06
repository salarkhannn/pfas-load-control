# Sample data provenance

The PDF itself is kept local and ignored by Git. Download it from the cited
source when running the real-report parser test or demonstrating the workflow.

## Michigan MiEnviro Blissfield WWTP biosolids report

`michigan-mienviro-blissfield-wwtp-biosolids-2025.pdf` is an unmodified copy
of a public analytical laboratory report hosted by Michigan EGLE's MiEnviro
Portal.

- Source: https://mienviro.michigan.gov/ncore/downloadfile/-7977885413807905515
- Retrieved: 2026-08-06
- SHA-256: `1e0876f3b4b4531731fe364850b3be7f813f9ecda66c8fe64b3c6c4f2caea0d6`
- Facility project: Blissfield WWTP, Michigan
- Laboratory: Merit Laboratories, Inc.
- Biosolids sample ID: `S70707.01`
- Collection date: 2025-01-22
- Reported basis: dry weight where applicable
- PFAS method: ASTM D7968-17M, modified isotopic dilution
- PFOS: 5.5 µg/kg dry weight
- PFOA: 3 µg/kg dry weight

The report is suitable for demonstrating Michigan's PFAS biosolids tier
screening. The resulting tier is not permission to land apply: an actual land
application also requires an approved RMP and compliance with all batch,
facility, field, agronomic, and site-specific requirements.

## Synthetic Module 6 branch fixtures

`synthetic-module-06-elevated.csv` and
`synthetic-module-06-prohibited.csv` are deterministic test fixtures, not
laboratory reports and not measurements from Blissfield WWTP or any other real
facility. They exist only to exercise the elevated and prohibited product
branches. Their EPA 1633 method, dry-weight basis, units, and required PFOS/PFOA
columns match the application's supported lab-report contract.
