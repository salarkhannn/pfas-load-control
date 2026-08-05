import { expect, test, type Page } from '@playwright/test';

const facility = { id: 'f29f83e9-7379-4db8-9192-620d7f0b5181', name: 'Great Lakes WRRF', jurisdiction: 'MI' };
const baseField = {
  id: '46691ea1-cf9f-40b4-a63c-20e89669d509',
  facility,
  name: 'North 40',
  locatorKind: 'ADDRESS',
  locatorInput: '100 Farm Road, Lansing, MI',
  status: 'NEEDS_LOCATION',
  details: {},
  gaps: [
    gap('LOCATION_UNRESOLVED', 'The field location has not been safely resolved.'),
    gap('GEOMETRY_UNCONFIRMED', 'The actual application boundary is not confirmed.'),
    gap('RMP_APPROVAL_MISSING', 'RMP approval is not confirmed.'),
    gap('USABLE_ACRES_MISSING', 'Usable application acres are missing.'),
    gap('AGRONOMIC_RATE_MISSING', "The operator's agronomic rate is missing."),
    gap('PRIOR_LOADING_MISSING', 'Prior biosolids loading is missing.'),
  ],
  createdAt: '2026-08-05T08:00:00Z',
  updatedAt: '2026-08-05T08:00:00Z',
};

const resolvedField = {
  ...baseField,
  status: 'NEEDS_GEOMETRY',
  location: {
    id: '4e1e3fae-c708-4864-b0fa-271b45c3eabc',
    disposition: 'resolved',
    latitude: '42.7335',
    longitude: '-84.5555',
    resolvedAddress: '100 Farm Road, Lansing, MI 48910',
    state: 'Michigan',
    county: 'Ingham County',
    confidence: '0.8',
    matchMethod: 'map_click+point_in_parcel',
    parcel: {
      id: 'P-100',
      geometry: { type: 'Polygon', coordinates: [[[-84.56, 42.73], [-84.55, 42.73], [-84.55, 42.74], [-84.56, 42.73]]] },
      matchType: 'exact_intersect',
      matchDistanceM: '0',
      source: 'regrid_paid',
    },
    parcelUnavailable: false,
    candidates: [],
    requestId: 'req_module_3',
    sourceUrl: 'https://api.mireye.com/v1/lookup',
    responseHash: 'a'.repeat(64),
    fetchedAt: '2026-08-05T08:01:00Z',
  },
  gaps: baseField.gaps.filter((item) => item.code !== 'LOCATION_UNRESOLVED'),
  updatedAt: '2026-08-05T08:01:00Z',
};

const boundedField = {
  ...resolvedField,
  status: 'NEEDS_DETAILS',
  geometry: {
    version: 1,
    source: 'MIREYE_PARCEL_CONFIRMED',
    geojson: resolvedField.location.parcel.geometry,
    areaAcres: '42.5',
    hash: 'b'.repeat(64),
    confirmedAt: '2026-08-05T08:02:00Z',
  },
  gaps: resolvedField.gaps.filter((item) => item.code !== 'GEOMETRY_UNCONFIRMED'),
  updatedAt: '2026-08-05T08:02:00Z',
};

const readyField = {
  ...boundedField,
  status: 'READY',
  details: {
    miEnviroSiteId: 'MI-100', rmpApproved: true, usableAcres: '42.5',
    agronomicRateDryTonsPerAcre: '2', priorLoadingDryTons: '0', cropOrUse: 'Corn',
  },
  gaps: [],
  updatedAt: '2026-08-05T08:03:00Z',
};

test('adds, resolves, confirms, and completes a candidate field', async ({ page }) => {
  await openBatchWorkspace(page);
  await mockFieldsAPI(page);
  await page.goto('/');

  await expect(page.getByRole('heading', { name: 'Candidate fields' })).toBeVisible();
  await page.getByLabel('Field name').fill('North 40');
  await page.getByRole('textbox', { name: 'Address' }).fill('100 Farm Road, Lansing, MI');
  await page.getByRole('button', { name: 'Add field' }).click();

  await expect(page.getByRole('heading', { name: 'North 40' })).toBeVisible();
  await expect(page.getByRole('region', { name: 'Parcel boundary map for 100 Farm Road, Lansing, MI 48910' })).toBeVisible();
  await expect(page.getByText('100 Farm Road, Lansing, MI 48910')).toBeVisible();
  await expect(page.getByText('Exact point match')).toBeVisible();

  await page.getByRole('button', { name: 'Confirm boundary' }).click();
  await expect(page.getByText('Boundary confirmed', { exact: true })).toBeVisible();

  await page.getByLabel('MiEnviro site ID').fill('MI-100');
  await page.getByLabel('Usable acres').fill('42.5');
  await page.getByLabel('Agronomic rate').fill('2');
  await page.getByLabel('Prior loading').fill('0');
  await page.getByLabel('Approved in the Residuals Management Program').check();
  await page.getByRole('button', { name: 'Save facts' }).click();

  await expect(page.getByText('Ready for screening')).toBeVisible();
  await expect(page.locator('.field-record__header').getByText('Ready', { exact: true })).toBeVisible();
});

test('checks a ready field and keeps every conclusion linked to its evidence', async ({ page }) => {
  await openBatchWorkspace(page);
  await mockFieldsAPI(page, [readyField]);
  await mockPhysicalEvidenceAPI(page);
  await page.goto('/');

  await expect(page.getByRole('heading', { name: 'Check physical conditions' })).toBeVisible();
  await page.getByRole('button', { name: 'Check field' }).click();

  await expect(page.getByRole('heading', { name: 'Physical evidence ready' })).toBeVisible();
  await expect(page.getByRole('region', { name: /with 5 evidence points/ })).toBeVisible();
  await expect(page.getByText('Floodplain intersects field')).toBeVisible();
  await page.getByText('Soil and slope').click();
  await expect(page.getByText('Slope', { exact: true })).toBeVisible();
  await expect(page.getByText('Michigan mapped wells')).toBeVisible();

  await page.getByText('Floodplain intersects field').click();
  await expect(page.getByRole('table', { name: 'Floodplain intersects field by field sample' })).toBeVisible();
  await expect(page.getByRole('link', { name: /FEMA National Flood Hazard Layer/ })).toHaveAttribute('href', 'https://www.fema.gov/flood-maps');

  await page.setViewportSize({ width: 320, height: 900 });
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  expect(overflow).toBeLessThanOrEqual(0);
});

test('keeps the field ledger usable at 320px', async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 900 });
  await openBatchWorkspace(page);
  await mockFieldsAPI(page, [resolvedField]);
  await page.goto('/');
  await expect(page.getByRole('heading', { name: 'Candidate fields' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'North 40' })).toBeVisible();
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  expect(overflow).toBeLessThanOrEqual(0);
});

async function mockFieldsAPI(page: Page, initial: typeof baseField[] = []) {
  let fields = [...initial];
  await page.route('http://localhost:8080/api/v1/field-context', (route) => route.fulfill(json({ facilities: [facility], fields })));
  await page.route(`http://localhost:8080/api/v1/candidate-fields/${baseField.id}/physical-evaluations/latest`, (route) => route.fulfill(json({ detail: 'Physical field evidence not found.' }, 404)));
  await page.route(`http://localhost:8080/api/v1/facilities/${facility.id}/candidate-fields`, (route) => {
    fields = [baseField];
    return route.fulfill(json(baseField, 201));
  });
  await page.route(`http://localhost:8080/api/v1/candidate-fields/${baseField.id}/resolution`, (route) => {
    fields = [resolvedField];
    return route.fulfill(json(resolvedField));
  });
  await page.route(`http://localhost:8080/api/v1/candidate-fields/${baseField.id}/parcel-confirmation`, (route) => {
    fields = [boundedField];
    return route.fulfill(json(boundedField));
  });
  await page.route(`http://localhost:8080/api/v1/candidate-fields/${baseField.id}/details`, (route) => {
    fields = [readyField];
    return route.fulfill(json(readyField));
  });
}

async function mockPhysicalEvidenceAPI(page: Page) {
  const evaluationId = 'f673b003-47e3-441a-97a2-dfa384fc0a79';
  const samples = [
    { index: 0, label: 'Anchor', latitude: 42.735, longitude: -84.555 },
    { index: 1, label: 'Interior 1', latitude: 42.732, longitude: -84.558 },
    { index: 2, label: 'Interior 2', latitude: 42.732, longitude: -84.552 },
    { index: 3, label: 'Interior 3', latitude: 42.738, longitude: -84.558 },
    { index: 4, label: 'Interior 4', latitude: 42.738, longitude: -84.552 },
  ];
  const base = {
    id: evaluationId,
    fieldId: readyField.id,
    geometryVersion: 1,
    fieldSetVersion: 'pfas-physical-v1',
    aggregationVersion: 'anchor-grid-2x2-v1',
    catalogVersion: '304-fields',
    sampleCount: 5,
    projectedCredits: 200,
    samples,
    facts: [],
    supplemental: [],
    gaps: [],
    createdAt: '2026-08-05T09:00:00Z',
    updatedAt: '2026-08-05T09:00:00Z',
  };
  const complete = {
    ...base,
    status: 'SUCCEEDED',
    completedAt: '2026-08-05T09:00:03Z',
    updatedAt: '2026-08-05T09:00:03Z',
    facts: [
      fact('within_floodplain_polygon', 'Floodplain intersects field', 'WATER', false, 'FEMA National Flood Hazard Layer', 'https://www.fema.gov/flood-maps', samples),
      fact('slope_degrees', 'Slope', 'SOIL', { min: 1.2, max: 3.8, median: 2.4 }, 'USGS 3DEP', 'https://www.usgs.gov/3d-elevation-program', samples, 'degree'),
    ],
    supplemental: [
      {
        provider: 'WELLOGIC', kind: 'MAPPED_WELLS', status: 'AVAILABLE', title: 'Michigan mapped wells',
        summary: '2 mapped wells were returned near the field; the nearest is 420 m from its boundary.',
        caveat: 'A mapped well is evidence of a known record. No result is not proof that no well exists.',
        sourceUrl: 'https://www.arcgis.com/home/item.html?id=58c98df11b6a411c97b8aeb839a695ad',
        fetchedAt: '2026-08-05T09:00:02Z',
      },
    ],
  };

  await page.route(`http://localhost:8080/api/v1/candidate-fields/${readyField.id}/physical-evaluations/latest`, (route) => route.fulfill(json({ detail: 'Physical field evidence not found.' }, 404)));
  await page.route(`http://localhost:8080/api/v1/candidate-fields/${readyField.id}/physical-evaluations`, (route) => route.fulfill(json({ evaluation: { ...base, status: 'QUEUED' }, created: true }, 202)));
  await page.route(`http://localhost:8080/api/v1/physical-evaluations/${evaluationId}`, (route) => route.fulfill(json(complete)));
}

function fact(
  name: string,
  label: string,
  category: string,
  value: unknown,
  source: string,
  sourceUrl: string,
  samples: Array<{ index: number; label: string; latitude: number; longitude: number }>,
  unit?: string,
) {
  return {
    name, label, category, value, unit, source, sourceUrl, state: 'COMPLETE', aggregateMethod: 'ANY_TRUE',
    fetchedAt: '2026-08-05T09:00:01Z', okCount: 5, absentCount: 0, failedCount: 0, critical: true,
    samples: samples.map((sample) => ({ ...sample, status: 'ok', value, unit, source, sourceUrl, fetchedAt: '2026-08-05T09:00:01Z' })),
  };
}

async function openBatchWorkspace(page: Page) {
  const reportId = '0012912b-4cf4-4213-a1ce-56ac5612062e';
  const report = {
    id: reportId,
    status: 'CONFIRMED',
    originalFilename: 'batch-42.csv',
    mediaType: 'text/csv',
    sizeBytes: 284,
    sha256: 'a'.repeat(64),
    facility,
    batch: { id: 'batch-id', identifier: 'BATCH-42', facilityId: facility.id, facilityName: facility.name, jurisdiction: 'MI' },
    pages: [], gaps: [], createdAt: '2026-08-05T07:00:00Z', updatedAt: '2026-08-05T07:01:00Z',
  };
  const decision = {
    id: 'decision-id', reportId, reportVersion: 1, facilityName: facility.name, batchIdentifier: 'BATCH-42', jurisdiction: 'MI', tier: 'STANDARD',
    explanation: 'PFOS and PFOA are both below 20 µg/kg dry weight.', matchedRuleId: 'MI-PFAS-STANDARD-BELOW-20', prohibitedActions: [],
    analytes: [
      { canonicalAnalyte: 'PFOS', resultText: '2.68', isNonDetect: false, normalizedValueUgKgDry: '2.68', sourcePage: 1 },
      { canonicalAnalyte: 'PFOA', resultText: 'ND', isNonDetect: true, upperBoundUgKgDry: '0.85', sourcePage: 1 },
    ],
    requirements: [],
    rulePack: {
      schemaVersion: 1, code: 'MI-PFAS-BIOSOLIDS', version: '2024.3', jurisdiction: 'MI', authorityType: 'INTERIM_POLICY',
      effectiveFrom: '2024-01-01', retrievedAt: '2026-08-05T00:00:00Z', sourceUrl: 'https://www.michigan.gov/egle', sourceTitle: 'Michigan EGLE PFAS biosolids interim strategy',
      reviewStatus: 'ACTIVE', reviewedBy: 'test', reviewedAt: '2026-08-05T00:00:00Z', checksum: 'b'.repeat(64), explanation: 'Michigan PFAS biosolids tier rules.',
      elevatedThresholdUgKgDry: '20', prohibitedThresholdUgKgDry: '100', acceptedAnalyticalMethodTokens: [], rules: [], requirements: [],
    },
    inputHash: 'c'.repeat(64), createdAt: '2026-08-05T07:01:01Z',
  };
  await page.addInitScript((id) => localStorage.setItem('pfas.current-report.v1', id), reportId);
  await page.route('http://localhost:8080/api/v1/lab-context', (route) => route.fulfill(json({ facilities: [facility], batches: [report.batch] })));
  await page.route(`http://localhost:8080/api/v1/lab-reports/${reportId}`, (route) => route.fulfill(json(report)));
  await page.route(`http://localhost:8080/api/v1/lab-reports/${reportId}/classification`, (route) => route.fulfill(json(decision)));
}

function gap(code: string, detail: string) {
  return { id: `${code}-id`, code, detail, resolution: `Resolve ${detail.toLowerCase()}`, createdAt: '2026-08-05T08:00:00Z' };
}

function json(body: unknown, status = 200) {
  return { status, contentType: 'application/json', headers: { 'access-control-allow-origin': 'http://127.0.0.1:4173' }, body: JSON.stringify(body) };
}
