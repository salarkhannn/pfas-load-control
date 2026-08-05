import { expect, test, type Page } from '@playwright/test';

const report = {
  id: '0012912b-4cf4-4213-a1ce-56ac5612062e',
  status: 'READY_TO_CONFIRM',
  originalFilename: 'batch-42.csv',
  mediaType: 'text/csv',
  sizeBytes: 284,
  sha256: 'a'.repeat(64),
  facility: { id: '1', name: 'Great Lakes WRRF', jurisdiction: 'MI' },
  batch: { id: '2', identifier: 'BATCH-42', facilityId: '1', facilityName: 'Great Lakes WRRF', jurisdiction: 'MI' },
  draft: {
    version: 1,
    status: 'DRAFT',
    source: 'EXTRACTION',
    laboratory: 'Great Lakes Lab',
    sampleIdentifier: 'BATCH-42',
    collectionDate: '2026-08-01',
    matrix: 'BIOSOLIDS',
    method: 'EPA 1633',
    basis: 'DRY',
    analytes: [
      analyte('PFOS', '19.99', false),
      analyte('PFOA', 'ND', true),
    ],
  },
  pages: [{ number: 1, text: 'PFOS,19.99,ug/kg\nPFOA,ND,ug/kg', extractionMethod: 'CSV' }],
  gaps: [],
  createdAt: '2026-08-05T07:00:00Z',
  updatedAt: '2026-08-05T07:00:01Z',
};

const decision = {
  id: 'f224ca7c-3b2f-4f9e-b54b-f39aa12af183',
  reportId: report.id,
  reportVersion: 1,
  facilityName: report.facility.name,
  batchIdentifier: report.batch.identifier,
  jurisdiction: 'MI',
  tier: 'STANDARD',
  explanation: 'PFOS and PFOA are both below 20 µg/kg dry weight.',
  matchedRuleId: 'MI-PFAS-STANDARD-BELOW-20',
  prohibitedActions: [],
  analytes: [
    { canonicalAnalyte: 'PFOS', resultText: '19.99', isNonDetect: false, normalizedValueUgKgDry: '19.99', sourcePage: 1 },
    { canonicalAnalyte: 'PFOA', resultText: 'ND', isNonDetect: true, upperBoundUgKgDry: '1', sourcePage: 1 },
  ],
  requirements: [
    {
      id: 'MI-PFAS-RESULT-SUBMISSION',
      title: 'Submit the sample results',
      detail: 'Submit PFAS sample results through MiEnviro before land application.',
      timing: 'At least two weeks before land application',
      ruleId: 'MI-PFAS-STANDARD-BELOW-20',
      sourceUrl: 'https://www.michigan.gov/egle/about/organization/water-resources/biosolids/pfas-related/interim-strategy',
      sourceTitle: 'Michigan EGLE PFAS biosolids interim strategy',
      authorityType: 'INTERIM_POLICY',
    },
  ],
  rulePack: {
    schemaVersion: 1,
    code: 'MI-PFAS-BIOSOLIDS',
    version: '2024.3',
    jurisdiction: 'MI',
    authorityType: 'INTERIM_POLICY',
    effectiveFrom: '2024-01-01',
    retrievedAt: '2026-08-05T00:00:00Z',
    sourceUrl: 'https://www.michigan.gov/egle/about/organization/water-resources/biosolids/pfas-related/interim-strategy',
    sourceTitle: 'Michigan EGLE PFAS biosolids interim strategy',
    reviewStatus: 'ACTIVE',
    reviewedBy: 'implementation-source-review-2026-08-05',
    reviewedAt: '2026-08-05T00:00:00Z',
    checksum: 'b'.repeat(64),
    explanation: 'Michigan PFAS biosolids tier rules.',
    elevatedThresholdUgKgDry: '20',
    prohibitedThresholdUgKgDry: '100',
    acceptedAnalyticalMethodTokens: ['1633', 'D7979', 'ISOTOPE DILUTION', '537 MODIFIED', '537M'],
    rules: [],
    requirements: [],
  },
  inputHash: 'c'.repeat(64),
  createdAt: '2026-08-05T07:01:01Z',
};

test('uploads, reviews, and confirms source-linked lab evidence', async ({ page }) => {
  await mockLabAPI(page);
  await page.goto('/');

  await page.getByLabel('Facility').fill('Great Lakes WRRF');
  await page.getByLabel('Batch').fill('BATCH-42');
  await page.locator('input[type="file"]').setInputFiles({
    name: 'batch-42.csv',
    mimeType: 'text/csv',
    buffer: Buffer.from('analyte,result,unit\nPFOS,19.99,ug/kg\nPFOA,ND,ug/kg'),
  });
  await page.getByRole('button', { name: 'Extract report' }).click();

  await expect(page.getByRole('heading', { name: 'BATCH-42' })).toBeVisible();
  await expect(page.getByText('PFOS', { exact: true })).toBeVisible();
  await expect(page.getByText('Page 1').first()).toBeVisible();
  await page.getByRole('button', { name: 'Confirm values' }).click();
  await expect(page.getByRole('heading', { name: 'Below PFAS action thresholds' })).toBeVisible();
  await page.getByText('View policy').click();
  await expect(page.getByText('Submit the sample results')).toBeVisible();
  await expect(page.getByRole('link', { name: 'Open source' })).toHaveAttribute('href', decision.rulePack.sourceUrl);
  await expect(page.getByRole('heading', { name: 'Candidate fields' })).toBeVisible();
});

test('keeps lab intake usable at 320px', async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 800 });
  await mockLabAPI(page);
  await page.goto('/');
  await expect(page.getByRole('heading', { name: 'Add a PFAS lab report' })).toBeVisible();
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  expect(overflow).toBeLessThanOrEqual(0);
});

async function mockLabAPI(page: Page) {
  await page.route('http://localhost:8080/api/v1/lab-context', (route) => route.fulfill(json({ facilities: [], batches: [] })));
  await page.route('http://localhost:8080/api/v1/lab-reports', (route) => route.fulfill(json({ report, created: true }, 201)));
  await page.route(`http://localhost:8080/api/v1/lab-reports/${report.id}/content`, (route) => route.fulfill({ status: 200, contentType: 'text/csv', body: report.pages[0].text }));
  await page.route(`http://localhost:8080/api/v1/lab-reports/${report.id}/confirmation`, (route) => route.fulfill(json({ ...report, status: 'CONFIRMED', confirmedAt: '2026-08-05T07:01:00Z' })));
  await page.route(`http://localhost:8080/api/v1/lab-reports/${report.id}/classification`, (route) => {
    if (route.request().method() === 'GET') return route.fulfill(json({ detail: 'Policy decision not found.' }, 404));
    return route.fulfill(json({ decision, created: true }, 201));
  });
  await page.route('http://localhost:8080/api/v1/field-context', (route) => route.fulfill(json({ facilities: [report.facility], fields: [] })));
}

function analyte(name: string, resultText: string, isNonDetect: boolean) {
  return {
    canonicalAnalyte: name,
    reportedAnalyte: name,
    resultText,
    value: isNonDetect ? undefined : resultText,
    unit: 'UG_KG',
    basis: 'DRY',
    isNonDetect,
    reportingLimit: isNonDetect ? '1' : undefined,
    sourcePage: 1,
    sourceExcerpt: `${name},${resultText},ug/kg`,
  };
}

function json(body: unknown, status = 200) {
  return { status, contentType: 'application/json', headers: { 'access-control-allow-origin': 'http://127.0.0.1:4173' }, body: JSON.stringify(body) };
}
