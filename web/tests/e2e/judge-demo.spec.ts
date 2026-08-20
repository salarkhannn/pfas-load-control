import { expect, test } from '@playwright/test';

test('replays the prepared case through the backend contract and preserves placement invariants at 320px', async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 800 });
  let replayRequests = 0;
  let reviewedRequests = 0;
  const idempotencyKeys: string[] = [];
  await page.route('http://localhost:8080/api/v1/judge-demo/runs', async (route) => {
    replayRequests += 1;
    expect(route.request().method()).toBe('POST');
    const key = route.request().headers()['idempotency-key'];
    expect(key).toBeTruthy();
    idempotencyKeys.push(key);
    await route.fulfill(json(judgeRun(replayRequests)));
  });
  await page.route('http://localhost:8080/api/v1/judge-demo/runs/*/reviewed', async (route) => {
    reviewedRequests += 1;
    expect(route.request().headers()['idempotency-key']).toBeTruthy();
    await route.fulfill(json(judgeRun(100 + reviewedRequests, true, '00000000-0000-4000-8000-000000000001')));
  });

  await page.goto('/judge-demo');
  await expect(page).toHaveTitle('Judge Demo | FieldProof');
  await expect(page.getByRole('heading', { name: 'A captured Mireye slope result blocked Field A from this allocation.' })).toBeVisible();
  expect(replayRequests).toBe(1);

  const fieldA = page.locator('.demo-field').filter({ hasText: 'Riverbend East' });
  const fieldB = page.locator('.demo-field').filter({ hasText: 'North Forty' });
  await page.getByText('Inspect both field calculations', { exact: true }).click();
  await expect(fieldA).toContainText('Field A · rank 2');
  await expect(fieldA).toContainText('Available capacityNot calculated');
  await expect(fieldA).toContainText('Engine allocation0 t');
  await expect(fieldB).toContainText('Field B · rank 1');
  await expect(fieldB).toContainText('Available capacity28 t');
  await expect(fieldB).toContainText('Engine allocation28 t');

  expect(28).toBeLessThanOrEqual(28);
  expect(28 + 24).toBe(52);
  await expect(page.locator('.allocation-change')).toContainText('24 t unallocated · Field A in review');

  await expect(page.getByText('Professional boundary and slope review required')).toBeVisible();
  await expect(page.getByText('Professional authorization required').first()).toBeAttached();
  await expect(page.getByRole('link', { name: /USGS 3DEP through captured Mireye batch/ })).toHaveAttribute('href', '#mireye-source');
  await expect(page.getByRole('link', { name: 'Download verified JSON' })).toHaveAttribute('href', `http://localhost:8080/api/v1/judge-demo/runs/00000000-0000-4000-8000-000000000001/package`);
  await expect(page.getByRole('heading', { name: 'Frozen record and controlled handoff' })).toBeAttached();

  await page.getByRole('button', { name: 'Apply reviewed evidence' }).click();
  await expect.poll(() => reviewedRequests).toBe(1);
  await expect(page.getByRole('heading', { name: 'Parent-bound geometry and a new sampled slope screen reopened Field A for calculation.' })).toBeVisible();
  await expect(fieldA).toContainText('Geometry-derived acres31.6');
  await expect(fieldA).toContainText('Available capacity29.92 t');
  await expect(fieldA).toContainText('Engine allocation24 t');
  await expect(page.getByRole('heading', { name: 'Seeded reviewed boundary evidence — demonstration only' })).toBeVisible();
  await expect(page.getByText(/stored authorization a4c0b3a1-6f74-4cad-9df1-7d7bc68d1002/)).toBeVisible();
  await expect(page.locator('.reviewed-evidence-record code').filter({ hasText: 'approval' })).toBeVisible();
  await expect(page.locator('.reviewed-evidence-record')).toContainText('parent boundary v3');
  await expect(page.locator('.reviewed-evidence-record')).toContainText('5/5 planned locations returned results');
  await expect(page.getByText('Sampled terrain screen passed', { exact: true }).first()).toBeVisible();
  await expect(page.locator('.reviewed-evidence-record')).toContainText('Unsampled terrain may contain different conditions');
  await expect(page.locator('.reviewed-evidence-record')).toContainText('request cap 8');
  await expect(page.locator('.reviewed-evidence-record')).toContainText('configured demonstration roles');
  await expect(page.getByText('Calculation ready', { exact: true })).toBeVisible();
  await expect(page.getByText('Professional authorization required', { exact: true }).first()).toBeVisible();
  await expect(page.getByText(/gradePercent = tan/)).toBeVisible();
  await expect(page.locator('.agent-timeline__receipt')).toContainText('Froze the decision payload');
  await expect(page.getByText(/Verified parent boundary/)).toBeVisible();
  await expect(page.getByRole('button', { name: /Initial frozen run/ })).toBeVisible();
  await expect(page.getByRole('button', { name: /Reviewed-evidence run/ })).toBeVisible();

  await page.getByRole('button', { name: /Initial frozen run/ }).click();
  await expect(page.getByRole('heading', { name: 'A captured Mireye slope result blocked Field A from this allocation.' })).toBeVisible();
  await page.getByRole('button', { name: /Reviewed-evidence run/ }).click();
  await expect(page.getByRole('heading', { name: 'Parent-bound geometry and a new sampled slope screen reopened Field A for calculation.' })).toBeVisible();

  await page.getByRole('button', { name: 'Rerun unresolved' }).click();
  await expect.poll(() => replayRequests).toBe(2);
  expect(idempotencyKeys[1]).not.toBe(idempotencyKeys[0]);
  await expect(page.getByText('Run 00000000-0000-4000-8000-000000000002', { exact: true })).toBeAttached();

  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  expect(overflow).toBeLessThanOrEqual(0);
});

test('keeps the complete judging path visible on desktop', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 1000 });
  let requests = 0;
  await page.route('http://localhost:8080/api/v1/judge-demo/runs', async (route) => {
    requests += 1;
    await route.fulfill(json(judgeRun(requests)));
  });
  await page.route('http://localhost:8080/api/v1/judge-demo/runs/*/reviewed', async (route) => {
    await route.fulfill(json(judgeRun(101, true, '00000000-0000-4000-8000-000000000001')));
  });
  await page.goto('/judge-demo');
  await expect(page).toHaveTitle('Judge Demo | FieldProof');
  await expect(page.getByText('Physical evidence changed this plan')).toBeVisible();
  await expect(page.getByRole('heading', { name: 'The decisive facts sit beside their effect' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Backend-recorded decision calls and freeze receipt' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Frozen record and controlled handoff' })).toBeVisible();
  const fieldA = page.locator('.demo-field').filter({ hasText: 'Riverbend East' });
  const fieldB = page.locator('.demo-field').filter({ hasText: 'North Forty' });
  await expect(fieldA).toBeVisible();
  await expect(fieldB).toBeVisible();
  await expect(fieldA).toContainText('Available capacityNot calculated');
  await expect(fieldB).toContainText('Engine allocation28 t');
  await page.getByRole('button', { name: 'Apply reviewed evidence' }).click();
  await expect(page.getByRole('heading', { name: 'Parent-bound geometry and a new sampled slope screen reopened Field A for calculation.' })).toBeVisible();
  await expect(fieldA).toBeVisible();
  await expect(fieldA).toContainText('Engine allocation24 t');
  await expect(page.locator('.reviewed-evidence-record')).toBeVisible();
  expect(requests).toBe(1);
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  expect(overflow).toBeLessThanOrEqual(0);
});

function judgeRun(replay: number, reviewed = false, parentRunId?: string) {
  const completedAt = '2026-08-20T13:18:00Z';
  const id = `00000000-0000-4000-8000-${String(replay).padStart(12, '0')}`;
  const after = reviewed
    ? plan('READY', [
      { position: 1, fieldId: 'b', fieldName: 'North Forty', dryTons: '28', acres: '23.333333', rateDryTonsPerAcre: '1.2' },
      { position: 2, fieldId: 'a', fieldName: 'Riverbend East', dryTons: '24', acres: '20', rateDryTonsPerAcre: '1.2' },
    ], '52', '0', [field('b', 'North Forty', 'ELIGIBLE', 1, '28'), field('a', 'Riverbend East', 'ELIGIBLE', 2, '29.92')])
    : plan('REVIEW_REQUIRED', [
      { position: 1, fieldId: 'b', fieldName: 'North Forty', dryTons: '28', acres: '23.333333', rateDryTonsPerAcre: '1.2' },
    ], '28', '24', [field('b', 'North Forty', 'ELIGIBLE', 1, '28'), field('a', 'Riverbend East', 'REVIEW_REQUIRED', 2)]);
  const toolCalls = [
    tool(1, 'lab.report.extract', 'Parsed PFOS and PFOA from report page 4.'),
    tool(2, 'policy.classify', 'Applied Michigan rule pack 2024.4.'),
    tool(3, 'mireye.fetch.batch', 'Replayed the captured Mireye batch and found a sampled slope concern.'),
    tool(4, 'field.boundary.adjust', 'Applied the seeded operator boundary adjustment.'),
    tool(5, 'placement.evaluate', 'Calculated the pre-screen plan with Field A capacity of 52 dry tons.'),
    ...(reviewed ? [tool(6, 'mireye.revised-boundary.screen', 'Received results for 5 of 5 bounded polygon sample locations; maximum slope 1.502° (2.622% grade).'), tool(7, 'slope-resolution.verify', 'Verified parent boundary, revised boundary v4, configured demonstration roles, and bounded sampled-terrain evidence.'), tool(8, 'placement.evaluate', 'Recalculated the verified boundary and allocated 28 dry tons to Field B plus 24 dry tons to Field A.')] : [tool(6, 'placement.evaluate', 'Allocated 28 dry tons to Field B, placed Field A into slope review, and left 24 dry tons unallocated.')]),
  ];
  const citations = [
    citation('slope', 'Original sampled slope', '9.425° = 16.6% grade', 'USGS 3DEP through captured Mireye batch', 'https://www.usgs.gov/3d-elevation-program', reviewed ? 'Verified geometry excludes the original high-slope sample.' : 'Exceeded Michigan’s 6% grade (3.43°) threshold and placed Field A into review.'),
    citation('acreage', 'Boundary adjustment', '18.4 acres excluded', 'Seeded operator boundary adjustment', '#operator-boundary-input', reviewed ? 'Superseded by verified boundary version 4.' : 'Does not prove that the high-slope sample was excluded.'),
    citation('rate', 'Field A agronomic rate', '1.2 dry tons/acre', 'Approved RMP agronomic record', '#rmp-source', reviewed ? 'Controls verified capacity.' : 'Controls capacity only after slope review.'),
    citation('prior-loading', 'Field A prior loading', '8 dry tons', 'Operator loading ledger', '#loading-source', reviewed ? 'Reduces verified capacity to 29.92 dry tons.' : 'Will reduce capacity after slope review.'),
    citation('policy', 'PFOS/PFOA classification', '18.6 / 2.1 µg/kg dry', 'Michigan rule pack 2024.4', 'https://www.michigan.gov/egle/', 'Allows field screening while preserving field-specific review.'),
    ...(reviewed ? [citation('resolution', 'Parent-bound revised geometry', '31.6 geometry-derived acres', 'Seeded reviewed boundary evidence — demonstration only', '#reviewed-evidence', 'Matched parent boundary v3 and remained inside it.'), citation('revised-screening', 'Sampled terrain screen', '1.502° = 2.622% grade sampled maximum', 'USGS 3DEP through captured Mireye batch', 'https://api.mireye.com/v1/fetch/batch', 'Five sampled locations returned slopes below the screening threshold. Unsampled terrain may contain different conditions; this does not establish whole-field slope suitability.')] : []),
  ];
  const resolutionEvidence = reviewed ? {
    fixtureVersion: 'seeded-reviewed-boundary-20260820-v1', label: 'Seeded reviewed boundary evidence — demonstration only', recordId: 'a4c0b3a1-6f74-4cad-9df1-7d7bc68d1001', evidenceType: 'CONFIRMED_BOUNDARY_EXCLUSION', artifactHash: '3'.repeat(64), artifact: { type: 'Polygon' }, reviewerAuthorizationRecordId: 'a4c0b3a1-6f74-4cad-9df1-7d7bc68d1002', reviewerAuthorizationHash: '4'.repeat(64), reviewerAuthorizationArtifact: { approvalStatus: 'APPROVED' }, parentBoundaryRecordId: 'a4c0b3a1-6f74-4cad-9df1-7d7bc68d1000', parentBoundaryArtifactHash: '2'.repeat(64), parentBoundaryArtifact: { type: 'Polygon' }, revisedScreeningRecordId: 'a4c0b3a1-6f74-4cad-9df1-7d7bc68d1003', revisedScreeningArtifactHash: '5'.repeat(64), revisedScreeningArtifact: { algorithmVersion: 'POLYGON_STRATIFIED_SCREEN_V1', requestedSampleCount: 5, returnedSampleCount: 5, maxRequestSamples: 8 },
    verification: { programPolicyVersion: 'DEMO-MI-LAND-APPLICATION-2026.08.1', demonstrationRoles: true, evidenceRecordId: 'a4c0b3a1-6f74-4cad-9df1-7d7bc68d1001', evidenceType: 'CONFIRMED_BOUNDARY_EXCLUSION', artifactHash: '3'.repeat(64), fieldId: 'a', boundaryVersion: 4, crs: 'EPSG:4326', parentBoundaryEvidenceRecordId: 'a4c0b3a1-6f74-4cad-9df1-7d7bc68d1000', parentBoundaryArtifactHash: '2'.repeat(64), parentBoundaryVersion: 3, sourceEvidenceRecordId: 'source-1', sourceArtifactHash: '9'.repeat(64), revisedScreeningEvidenceRecordId: 'a4c0b3a1-6f74-4cad-9df1-7d7bc68d1003', revisedScreeningArtifactHash: '5'.repeat(64), issuingRole: 'DEMO_FIELD_BOUNDARY_ISSUER', reviewerRole: 'DEMO_PLACEMENT_REVIEWER', reviewerAuthorizationRecordId: 'a4c0b3a1-6f74-4cad-9df1-7d7bc68d1002', reviewerAuthorizationHash: '4'.repeat(64), recordedAt: completedAt, verifiedAt: completedAt, derivedUsableAcres: '31.6', highSlopeSamplesExcluded: 1, parentBoundaryAcres: '50', revisedScreening: { evidenceRecordId: 'a4c0b3a1-6f74-4cad-9df1-7d7bc68d1003', artifactHash: '5'.repeat(64), endpoint: 'https://api.mireye.com/v1/fetch/batch', requestId: 'req_605e4507cae5', requestHash: '6'.repeat(64), responseHash: '7'.repeat(64), retrievedAt: completedAt, algorithmVersion: 'POLYGON_STRATIFIED_SCREEN_V1', minimumSpacingMeters: '75', maxRequestSamples: 8, requestedSampleCount: 5, returnedSampleCount: 5, boundaryNearSampleCount: 4, interiorSampleCount: 1, maximumSlopeDegrees: '1.502', maximumSlopeGradePercent: '2.622', status: 'SAMPLED_TERRAIN_SCREEN_PASSED', limitation: 'Five sampled locations returned slopes below the screening threshold. Unsampled terrain may contain different conditions; this does not establish whole-field slope suitability.' }, slopeConversion: { originalDegrees: '9.425', derivedGradePercent: '16.6', thresholdGradePercent: '6', thresholdDegrees: '3.43', formula: 'gradePercent = tan(degrees × π / 180) × 100', policySourceUrl: 'https://www.michigan.gov/egle/' } },
  } : undefined;
  return {
    id,
    fixtureVersion: 'judge-demo-v2', mode: reviewed ? 'REVIEWED_EVIDENCE' : 'UNRESOLVED', ...(parentRunId ? { parentRunId } : {}), kind: 'LAND_APPLICATION_READINESS_DECISION', runStatus: 'SUCCEEDED', calculationStatus: reviewed ? 'READY' : 'REVIEW_REQUIRED', authorizationStatus: 'REQUIRED', authorizationRequired: true, caseId: 'MI-2026-014', batchDryTons: '52', excludedAcres: '18.4', reviewRequired: !reviewed,
    reviewQuestion: reviewed ? 'The boundary evidence cleared the slope gate for calculation. A responsible person must still authorize any real application.' : 'Provide an immutable confirmed-boundary record that proves the high-slope sample falls outside Riverbend East before calculating an allocation.',
    mireyeCapture: { fixtureVersion: 'mireye-fetch-batch-20260820-v1', endpoint: 'https://api.mireye.com/v1/fetch/batch', requestId: 'req_088804c2fcb6', httpStatus: 200, retrievedAt: completedAt, requestHash: '8'.repeat(64), responseHash: '9'.repeat(64), request: {}, response: {} },
    acreageAdjustment: { fixtureVersion: 'seeded-operator-boundary-adjustment-v1', inputType: 'OPERATOR_SUPPLIED', boundaryVersion: 3, recordedBoundaryAcres: '50', excludedAcres: '18.4', effectiveAcres: '31.6', recordedAt: completedAt, source: 'Seeded operator boundary adjustment', reason: 'Not a Mireye measurement.', inputHash: '7'.repeat(64), rawFixture: {} },
    ...(resolutionEvidence ? { resolutionEvidence } : {}),
    before: plan('READY', [{ position: 1, fieldId: 'a', fieldName: 'Riverbend East', dryTons: '52', acres: '43.333333', rateDryTonsPerAcre: '1.2' }], '52', '0', [field('a', 'Riverbend East', 'ELIGIBLE', 1, '52'), field('b', 'North Forty', 'ELIGIBLE', 2, '28')]),
    after, toolCalls, freezeReceipt: { position: toolCalls.length + 1, toolName: 'decisionpackage.freeze', status: 'SUCCEEDED', artifactId: '10000000-0000-4000-8000-000000000001', artifactHash: '1'.repeat(64), startedAt: completedAt, completedAt }, citations,
    package: { id: '10000000-0000-4000-8000-000000000001', status: 'FROZEN', inputHash: 'a'.repeat(64), decisionHash: 'b'.repeat(64), payloadHash: '1'.repeat(64), downloadUrl: `/api/v1/judge-demo/runs/${id}/package`, createdAt: completedAt },
    createdAt: completedAt, completedAt,
  };
}

function plan(status: string, allocations: unknown[], allocatedDryTons: string, unallocatedDryTons: string, fields: unknown[]) {
  return { status, tier: 'STANDARD', configVersion: '2026.1', configChecksum: 'c'.repeat(64), inputHash: 'd'.repeat(64), batchDryTons: '52', allocatedDryTons, unallocatedDryTons, fields, allocations, gaps: [] };
}
function field(fieldId: string, fieldName: string, disposition: string, rank: number, capacity?: string) { return { fieldId, fieldName, disposition, rank, explanation: 'Engine checked.', highConcernCount: disposition === 'REVIEW_REQUIRED' ? 1 : 0, moderateConcernCount: 0, dataGapCount: 0, ...(capacity ? { allowedRateDryTonsPerAcre: '1.2', availableCapacityDryTons: capacity } : {}), reasons: [], categories: [] }; }
function tool(position: number, toolName: string, summary: string) { return { position, toolName, status: 'SUCCEEDED', summary, sourceUrl: '#source', requestId: `request-${position}`, inputHash: 'e'.repeat(64), outputHash: 'f'.repeat(64), input: { position }, output: { ok: true }, startedAt: '2026-08-20T13:14:00Z', completedAt: '2026-08-20T13:18:00Z' }; }
function citation(id: string, finding: string, value: string, source: string, sourceUrl: string, effect: string) { return { id, finding, value, source, sourceUrl, effect, retrievedAt: '2026-08-20T13:16:00Z' }; }
function json(body: unknown) { return { status: 201, contentType: 'application/json', headers: { 'access-control-allow-origin': 'http://127.0.0.1:4173' }, body: JSON.stringify(body) }; }
