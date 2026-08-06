import { expect, test, type Page } from '@playwright/test';

const facility = { id: '3f3be0c0-dbd2-4d32-ab4b-04c31f816837', name: 'Lakeside WRRF', jurisdiction: 'MI' };
const reportId = '76e934ee-b7b3-424d-b840-182035cb4dce';
const decisionId = '27793314-ea54-482d-826b-06c8fc7e5300';
const locationId = '40db42cb-61a3-475a-97b6-8c1536f72eed';
const runId = 'e03dfd4f-6a89-4ae7-b808-02e777cb98e0';

test('builds an elevated source-reduction response without disposal candidates', async ({ page }) => {
  await mockBatch(page, 'ELEVATED');
  await mockResponse(page, 'ELEVATED');
  await page.goto('/');

  await expect(page.getByRole('heading', { name: 'Source investigation' })).toBeVisible();
  await page.getByRole('textbox', { name: 'Treatment plant address' }).fill('123 Water Street, Lansing, MI');
  await page.getByRole('button', { name: 'Find facility' }).click();
  await expect(page.getByText('123 Water St, Lansing, MI 48910')).toBeVisible();
  await page.getByRole('button', { name: 'Confirm & build response' }).click();

  await expect(page.getByRole('heading', { name: 'Source-reduction response ready' })).toBeVisible();
  await expect(page.getByText('Keep the PFAS rate ceiling in the placement plan', { exact: true })).toBeVisible();
  await expect(page.getByText('Potential Metal Finisher')).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Facilities to contact' })).toHaveCount(0);
});

test('blocks land application and prepares unverified alternatives for a prohibited batch', async ({ page }) => {
  await mockBatch(page, 'PROHIBITED');
  await mockResponse(page, 'PROHIBITED');
  await page.goto('/');

  await expect(page.getByRole('heading', { name: 'Control and investigation' })).toBeVisible();
  await page.getByRole('textbox', { name: 'Treatment plant address' }).fill('123 Water Street, Lansing, MI');
  await page.getByRole('button', { name: 'Find facility' }).click();
  await page.getByRole('button', { name: 'Confirm & build response' }).click();

  await expect(page.getByRole('heading', { name: 'Land application blocked; response ready' })).toBeVisible();
  await expect(page.getByText('Keep this batch out of land application', { exact: true })).toBeVisible();
  await expect(page.getByText('Notify EGLE through MiEnviro', { exact: true })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Facilities to contact' })).toBeVisible();
  await expect(page.getByText('Acceptance unverified')).toBeVisible();
  await expect(page.getByText('No facility has been contacted.')).toBeVisible();

  await page.setViewportSize({ width: 320, height: 900 });
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  expect(overflow).toBeLessThanOrEqual(0);
});

test('freezes the completed workflow into a reviewable package and downloads its PDF', async ({ page }) => {
  await mockBatch(page, 'ELEVATED');
  await mockResponse(page, 'ELEVATED');
  await mockDecisionPackage(page);
  await page.goto('/');

  await page.getByRole('textbox', { name: 'Treatment plant address' }).fill('123 Water Street, Lansing, MI');
  await page.getByRole('button', { name: 'Find facility' }).click();
  await page.getByRole('button', { name: 'Confirm & build response' }).click();
  await expect(page.getByRole('heading', { name: 'Source-reduction response ready' })).toBeVisible();

  await page.getByRole('button', { name: 'Generate package' }).click();
  await expect(page.getByRole('heading', { name: 'Decision package ready' })).toBeVisible();
  await expect(page.getByText('Review the proposed field allocation', { exact: true })).toBeVisible();
  await expect(page.getByText('Human review only. This package does not approve, submit, schedule, notify, contact, or execute any action.')).toBeVisible();

  const downloadStarted = page.waitForEvent('download');
  await page.getByRole('button', { name: /PDF/ }).click();
  await expect((await downloadStarted).suggestedFilename()).toBe('pfas-decision-package.pdf');

  await page.setViewportSize({ width: 320, height: 900 });
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  expect(overflow).toBeLessThanOrEqual(0);
});

async function mockBatch(page: Page, tier: 'ELEVATED' | 'PROHIBITED') {
  const report = {
    id: reportId, status: 'CONFIRMED', originalFilename: 'pfas.csv', mediaType: 'text/csv', sizeBytes: 200,
    sha256: 'a'.repeat(64), facility,
    batch: { id: 'batch-id', identifier: 'PFAS-26-07', facilityId: facility.id, facilityName: facility.name, jurisdiction: 'MI' },
    pages: [], gaps: [], createdAt: '2026-08-06T08:00:00Z', updatedAt: '2026-08-06T08:01:00Z',
  };
  const prohibited = tier === 'PROHIBITED';
  const decision = {
    id: decisionId, reportId, batchId: 'batch-id', reportVersion: 1, facilityName: facility.name,
    batchIdentifier: 'PFAS-26-07', jurisdiction: 'MI', tier,
    explanation: prohibited ? 'PFOS is at or above 100 µg/kg dry weight.' : 'PFOS is between 20 and 100 µg/kg dry weight.',
    matchedRuleId: prohibited ? 'MI-PFAS-PROHIBITED-100' : 'MI-PFAS-ELEVATED-20',
    prohibitedActions: prohibited ? ['LAND_APPLICATION'] : [],
    maximumApplicationRateDryTonsPerAcre: prohibited ? undefined : '1.5',
    analytes: [
      { canonicalAnalyte: 'PFOS', resultText: prohibited ? '118' : '48', isNonDetect: false, normalizedValueUgKgDry: prohibited ? '118' : '48', sourcePage: 1 },
      { canonicalAnalyte: 'PFOA', resultText: '4', isNonDetect: false, normalizedValueUgKgDry: '4', sourcePage: 1 },
    ],
    requirements: [],
    rulePack: {
      schemaVersion: 1, code: 'MI-PFAS-BIOSOLIDS', version: '2024.4', jurisdiction: 'MI', authorityType: 'INTERIM_POLICY',
      effectiveFrom: '2024-01-01', retrievedAt: '2026-08-06T00:00:00Z', sourceUrl: 'https://www.michigan.gov/egle/about/organization/water-resources/biosolids/pfas-related/interim-strategy', sourceTitle: 'Michigan EGLE PFAS biosolids interim strategy',
      reviewStatus: 'ACTIVE', reviewedBy: 'test', reviewedAt: '2026-08-06T00:00:00Z', checksum: 'b'.repeat(64), explanation: 'Michigan PFAS biosolids tier rules.',
      elevatedThresholdUgKgDry: '20', prohibitedThresholdUgKgDry: '100', acceptedAnalyticalMethodTokens: [], rules: [], requirements: [],
    },
    inputHash: 'c'.repeat(64), createdAt: '2026-08-06T08:02:00Z',
  };
  await page.addInitScript((id) => localStorage.setItem('pfas.current-report.v1', id), reportId);
  await page.route('http://localhost:8080/api/v1/lab-context', (route) => route.fulfill(json({ facilities: [facility], batches: [report.batch] })));
  await page.route(`http://localhost:8080/api/v1/lab-reports/${reportId}`, (route) => route.fulfill(json(report)));
  await page.route(`http://localhost:8080/api/v1/lab-reports/${reportId}/classification`, (route) => route.fulfill(json(decision)));
  await page.route('http://localhost:8080/api/v1/field-context', (route) => route.fulfill(json({ facilities: [facility], fields: [] })));
  await page.route(`http://localhost:8080/api/v1/policy-decisions/${decisionId}/placement/latest`, (route) => route.fulfill(json({ detail: 'Placement plan not found.' }, 404)));
  await page.route(`http://localhost:8080/api/v1/policy-decisions/${decisionId}/decision-packages/latest`, (route) => route.fulfill(json({ detail: 'Decision package not found.' }, 404)));
}

async function mockResponse(page: Page, tier: 'ELEVATED' | 'PROHIBITED') {
  const location = {
    id: locationId, facilityId: facility.id, input: '123 Water Street, Lansing, MI', kind: 'address', disposition: 'resolved',
    latitude: 42.733, longitude: -84.555, resolvedAddress: '123 Water St, Lansing, MI 48910', state: 'MI', county: 'Ingham', confidence: 0.97,
    candidates: [], sourceUrl: 'https://api.mireye.com/v1/lookup', fetchedAt: '2026-08-06T08:03:00Z', confirmed: false,
  };
  const common = {
    id: runId, decisionId, facilityLocationId: locationId, facilityName: facility.name, batchIdentifier: 'PFAS-26-07', tier,
    status: 'READY', policySourceUrl: 'https://www.michigan.gov/egle/about/organization/water-resources/biosolids/pfas-related/interim-strategy', policyVersion: '2024.4',
    location: { ...location, confirmed: true },
    investigationLeads: [{ position: 1, registryId: '110000000001', facilityName: 'Potential Metal Finisher', city: 'Lansing', state: 'MI', naicsCodes: ['332813'], evidenceTier: 3, evidenceLabel: 'Potential PFAS-handling sector', rationale: 'EPA ECHO lists this regulated facility within five miles under a potential PFAS-handling industry code.', caveat: 'Verify current operations and sewer connectivity.', sourceUrl: 'https://echo.epa.gov/detailed-facility-report?fid=110000000001' }],
    evidence: [{ provider: 'MIREYE', kind: 'SEWER_CONTEXT', status: 'AVAILABLE', title: 'Wastewater service context', summary: 'Screening context for the confirmed plant.', data: {}, sourceUrl: 'https://api.mireye.com/v1/batch', fetchedAt: '2026-08-06T08:04:00Z', caveat: 'Context does not prove a sewer connection.' }],
    dataGaps: [], failureCode: undefined, failureDetail: undefined, createdAt: '2026-08-06T08:04:00Z', updatedAt: '2026-08-06T08:04:02Z',
  };
  const elevatedTasks = [
    task(1, 'ENFORCE_RATE_CAP', 'Keep the PFAS rate ceiling in the placement plan', 'ENFORCED'),
    task(2, 'SOURCE_EFFLUENT_SAMPLE', 'Collect a source-effluent sample', 'REQUIRED'),
  ];
  const prohibitedTasks = [
    task(1, 'BLOCK_LAND_APPLICATION', 'Keep this batch out of land application', 'ENFORCED'),
    task(2, 'NOTIFY_EGLE', 'Notify EGLE through MiEnviro', 'REQUIRED'),
    task(3, 'ARRANGE_ALTERNATIVE_MANAGEMENT', 'Request acceptance and quotes', 'DRAFT'),
  ];
  const run = {
    ...common,
    tasks: tier === 'PROHIBITED' ? prohibitedTasks : elevatedTasks,
    alternatives: tier === 'PROHIBITED' ? [{ position: 1, wdsId: 'WDS-1', facilityName: 'Capital Area Landfill', facilityType: 'Type II MSW Landfill', address: '500 Disposal Road', city: 'Lansing', county: 'Ingham', latitude: 42.7, longitude: -84.6, disposalAreaStatus: 'Active - Accepting', straightlineDistanceKm: 12, routeStatus: 'ROUTED', drivingDistanceKm: 18.2, durationMinutes: 24, acceptanceStatus: 'UNVERIFIED', executable: false, sourceUrl: 'https://gis-egle.hub.arcgis.com/' }] : [],
  };
  await page.route(`http://localhost:8080/api/v1/policy-decisions/${decisionId}/response/latest`, (route) => route.fulfill(json({ detail: 'PFAS response not found.' }, 404)));
  await page.route(`http://localhost:8080/api/v1/policy-decisions/${decisionId}/facility-location/latest`, (route) => route.fulfill(json({ detail: 'PFAS response not found.' }, 404)));
  await page.route(`http://localhost:8080/api/v1/policy-decisions/${decisionId}/facility-location`, (route) => route.fulfill(json(location, 201)));
  await page.route(`http://localhost:8080/api/v1/facility-locations/${locationId}/confirmation`, (route) => route.fulfill(json({ ...location, confirmed: true })));
  await page.route(`http://localhost:8080/api/v1/policy-decisions/${decisionId}/response`, (route) => route.fulfill(json({ run: { ...run, status: 'QUEUED' }, created: true }, 201)));
  await page.route(`http://localhost:8080/api/v1/response-runs/${runId}`, (route) => route.fulfill(json(run)));
}

async function mockDecisionPackage(page: Page) {
  const packageId = '0b2ad249-49d1-4934-8681-dd8509a13eac';
  const value = {
    id: packageId,
    decisionId,
    schemaVersion: 'module-07.1',
    status: 'READY',
    inputHash: 'd'.repeat(64),
    snapshot: {
      decision: { id: decisionId },
      lab: { reportId, reportVersion: 1, originalFilename: 'pfas.csv', mediaType: 'text/csv', sha256: 'a'.repeat(64), analytes: [], gaps: [] },
      gaps: [],
    },
    evidence: [
      { position: 1, kind: 'LAB_REPORT', provider: 'Laboratory report', title: 'Confirmed PFAS laboratory evidence', status: 'AVAILABLE', detail: 'The confirmed source report is frozen in this package.' },
      { position: 2, kind: 'POLICY_RULE_PACK', provider: 'Michigan EGLE', title: 'Michigan EGLE PFAS biosolids interim strategy', status: 'AVAILABLE', detail: 'Rule MI-PFAS-ELEVATED-20 produced the ELEVATED batch tier.', sourceUrl: 'https://www.michigan.gov/egle/' },
    ],
    proposedActions: [
      { position: 1, code: 'REVIEW_DRAFT_ALLOCATION', category: 'PLACEMENT', state: 'DRAFT', title: 'Review the proposed field allocation', detail: 'Review the allocation against the approved Residuals Management Program before scheduling.', timing: 'Before scheduling', sourceId: 'plan-id', executable: false },
    ],
    artifacts: [
      { format: 'html', mediaType: 'text/html', sha256: 'e'.repeat(64), sizeBytes: 1024, url: `/api/v1/decision-packages/${packageId}/exports/html` },
      { format: 'pdf', mediaType: 'application/pdf', sha256: 'f'.repeat(64), sizeBytes: 2048, url: `/api/v1/decision-packages/${packageId}/exports/pdf` },
      { format: 'json', mediaType: 'application/json', sha256: '0'.repeat(64), sizeBytes: 512, url: `/api/v1/decision-packages/${packageId}/exports/json` },
    ],
    createdAt: '2026-08-06T09:00:00Z',
  };
  await page.route(`http://localhost:8080/api/v1/policy-decisions/${decisionId}/decision-packages`, (route) => route.fulfill(json({ package: value, created: true }, 201)));
  await page.route(`http://localhost:8080/api/v1/decision-packages/${packageId}/exports/pdf`, (route) => route.fulfill({
    status: 200,
    contentType: 'application/pdf',
    headers: {
      'access-control-allow-origin': 'http://127.0.0.1:4173',
      'access-control-expose-headers': 'Content-Disposition',
      'content-disposition': 'attachment; filename="pfas-decision-package.pdf"',
    },
    body: '%PDF-1.4\n%%EOF',
  }));
}

function task(position: number, code: string, title: string, state: string) {
  return { position, code, category: 'RESPONSE', title, detail: `${title}.`, timing: 'Before the next decision', state };
}

function json(body: unknown, status = 200) {
  return { status, contentType: 'application/json', headers: { 'access-control-allow-origin': 'http://127.0.0.1:4173' }, body: JSON.stringify(body) };
}
