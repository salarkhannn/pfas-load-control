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
    source: 'MIREYE_PARCEL',
    geojson: resolvedField.location.parcel.geometry,
    areaAcres: '42.5',
    hash: 'b'.repeat(64),
    confirmed: true,
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

const uploadedField = {
  ...baseField,
  name: '06S05E18-DW01',
  locatorKind: 'GEOJSON',
  locatorInput: 'Uploaded field boundary',
  status: 'NEEDS_GEOMETRY',
  geometry: {
    version: 1,
    source: 'UPLOADED_GEOJSON',
    geojson: resolvedField.location.parcel.geometry,
    areaAcres: '47.741295983',
    hash: 'c'.repeat(64),
    confirmed: false,
  },
  gaps: baseField.gaps.filter((item) => item.code !== 'LOCATION_UNRESOLVED'),
  updatedAt: '2026-08-05T08:02:00Z',
};

const confirmedUploadedField = {
  ...uploadedField,
  status: 'NEEDS_DETAILS',
  geometry: {
    ...uploadedField.geometry,
    confirmed: true,
    confirmedAt: '2026-08-05T08:03:00Z',
  },
  gaps: uploadedField.gaps.filter((item) => item.code !== 'GEOMETRY_UNCONFIRMED'),
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

test('keeps an uploaded boundary estimated until the operator confirms it', async ({ page }) => {
  await openBatchWorkspace(page);
  await mockUploadedBoundaryAPI(page);
  await page.goto('/');

  await page.getByLabel('Field name').fill('06S05E18-DW01');
  await page.getByLabel('Boundary file', { exact: true }).check();
  await page.getByLabel(/Choose a boundary file/).setInputFiles({
    name: 'estimated-boundary.geojson',
    mimeType: 'application/geo+json',
    buffer: Buffer.from(JSON.stringify(resolvedField.location.parcel.geometry)),
  });
  await page.getByRole('button', { name: 'Add field' }).click();

  await expect(page.getByText('47.74 acres uploaded')).toBeVisible();
  await expect(page.getByText('Confirm the application field')).toBeVisible();
  await expect(page.locator('.field-record__header').getByText('Boundary needed', { exact: true })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Check physical conditions' })).toHaveCount(0);

  await page.getByRole('button', { name: 'Confirm actual boundary' }).click();
  await expect(page.getByText('Boundary confirmed', { exact: true })).toBeVisible();
  await expect(page.getByText('Uploaded outline confirmed by the operator')).toBeVisible();

  await page.setViewportSize({ width: 320, height: 900 });
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  expect(overflow).toBeLessThanOrEqual(0);
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

test('compares eligible fields and preserves an exact draft allocation', async ({ page }) => {
  await openBatchWorkspace(page);
  await mockFieldsAPI(page, [readyField]);
  await mockPlacementAPI(page);
  await page.goto('/');

  await page.getByRole('button', { name: 'Build draft plan' }).click();
  await expect(page.getByRole('heading', { name: 'The batch fits' })).toBeVisible();
  await expect(page.getByText('North 40', { exact: true }).last()).toBeVisible();
  await expect(page.getByLabel('Proposed allocation').getByText('11.023 dry tons', { exact: true })).toBeVisible();
  await page.locator('.placement-field-row summary').click();
  await expect(page.getByText('Required field and evidence gates are complete.')).toBeVisible();
  await expect(page.getByRole('link', { name: /EPA draft guidance/ }).first()).toHaveAttribute('href', /epa\.gov/);

  await page.setViewportSize({ width: 320, height: 900 });
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  expect(overflow).toBeLessThanOrEqual(0);
});

test('explains when field review, rather than capacity, prevents allocation', async ({ page }) => {
  await openBatchWorkspace(page);
  await mockFieldsAPI(page, [readyField]);
  await mockPlacementAPI(page, 'REVIEW_REQUIRED');
  await page.goto('/');

  await page.getByRole('button', { name: 'Build draft plan' }).click();
  await expect(page.getByRole('heading', { name: 'Field evidence needs review' })).toBeVisible();
  await expect(page.getByText('No field is currently eligible to receive this batch.')).toBeVisible();
  await expect(page.getByRole('heading', { name: /capacity is needed/i })).toHaveCount(0);
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

async function mockUploadedBoundaryAPI(page: Page) {
  let fields: typeof uploadedField[] = [];
  await page.route('http://localhost:8080/api/v1/field-context', (route) => route.fulfill(json({ facilities: [facility], fields })));
  await page.route(`http://localhost:8080/api/v1/facilities/${facility.id}/candidate-fields`, (route) => {
    fields = [uploadedField];
    return route.fulfill(json(uploadedField, 201));
  });
  await page.route(`http://localhost:8080/api/v1/candidate-fields/${baseField.id}/boundary-confirmation`, (route) => {
    fields = [confirmedUploadedField];
    return route.fulfill(json(confirmedUploadedField));
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

async function mockPlacementAPI(page: Page, status: 'READY' | 'REVIEW_REQUIRED' = 'READY') {
  const reviewRequired = status === 'REVIEW_REQUIRED';
  const categories = [
    vulnerability('WATER_RECEPTORS', 'Water receptors', reviewRequired ? 'HIGH' : 'LOW'),
    vulnerability('SUBSURFACE_MOBILITY', 'Subsurface mobility', 'LOW'),
    vulnerability('SURFACE_TRANSPORT', 'Surface transport', reviewRequired ? 'HIGH' : 'LOW'),
    vulnerability('HUMAN_FOOD_EXPOSURE', 'Human and food exposure', 'MODERATE'),
    vulnerability('DATA_UNCERTAINTY', 'Evidence uncertainty', reviewRequired ? 'HIGH' : 'LOW'),
  ];
  const plan = {
    id: '49c45c39-cbde-4c7b-96c5-a7b27fa74a6b', decisionId: 'f224ca7c-3b2f-4f9e-b54b-f39aa12af183',
    status, tier: 'STANDARD', configVersion: 'mi-pfas-field-comparison-2026.08.1',
    configChecksum: 'd'.repeat(64), inputHash: 'e'.repeat(64), wetMassKg: '10000', percentSolids: '100',
    batchDryTons: '11.023113', allocatedDryTons: reviewRequired ? '0' : '11.023113', unallocatedDryTons: reviewRequired ? '11.023113' : '0',
    fields: [{
      fieldId: readyField.id, fieldName: readyField.name, disposition: reviewRequired ? 'REVIEW_REQUIRED' : 'ELIGIBLE', rank: reviewRequired ? undefined : 1,
      explanation: reviewRequired ? 'Critical physical evidence is incomplete or needs human review.' : 'Required field and evidence gates are complete.',
      counterfactual: reviewRequired ? undefined : 'This field is preferred under the current confirmed evidence.',
      highConcernCount: reviewRequired ? 3 : 0, moderateConcernCount: 1, dataGapCount: reviewRequired ? 1 : 0,
      allowedRateDryTonsPerAcre: reviewRequired ? undefined : '2', availableCapacityDryTons: reviewRequired ? undefined : '85',
      reasons: [reviewRequired ? 'Critical physical evidence is incomplete or needs human review.' : 'Required field and evidence gates are complete.'], categories,
    }],
    allocations: reviewRequired ? [] : [{ position: 1, fieldId: readyField.id, fieldName: readyField.name, dryTons: '11.023113', acres: '5.511557', rateDryTonsPerAcre: '2' }],
    gaps: reviewRequired ? [{ code: 'FIELD_REVIEW_REQUIRED', detail: 'No field is currently eligible to receive this batch.', resolution: 'Resolve the listed field evidence, or add another approved field, then compare again.' }] : [],
    createdAt: '2026-08-06T07:00:00Z',
  };
  await page.route('http://localhost:8080/api/v1/policy-decisions/*/placement/latest', (route) => route.fulfill(json({ detail: 'Placement plan not found.' }, 404)));
  await page.route('http://localhost:8080/api/v1/policy-decisions/*/placement', (route) => route.fulfill(json({ evaluation: plan, created: true }, 201)));
}

function vulnerability(key: string, label: string, band: string) {
  return {
    key, label, band, explanation: `${label} evidence is suitable for comparison.`, components: [],
    authorityType: 'DRAFT_GUIDANCE', sourceTitle: 'EPA draft guidance for reducing PFOA and PFOS risk in biosolids',
    sourceUrl: 'https://www.epa.gov/system/files/documents/2026-06/draft-guidance-reducing-risk-pfoa-pfos-biosolids.pdf',
    configVersion: 'mi-pfas-field-comparison-2026.08.1',
  };
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
    id: 'f224ca7c-3b2f-4f9e-b54b-f39aa12af183', reportId, batchId: 'batch-id', reportVersion: 1, facilityName: facility.name, batchIdentifier: 'BATCH-42', jurisdiction: 'MI', tier: 'STANDARD',
    wetMassKg: '10000', percentSolids: '100',
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
  await page.route(`http://localhost:8080/api/v1/policy-decisions/${decision.id}/placement/latest`, (route) => route.fulfill(json({ detail: 'Placement plan not found.' }, 404)));
}

function gap(code: string, detail: string) {
  return { id: `${code}-id`, code, detail, resolution: `Resolve ${detail.toLowerCase()}`, createdAt: '2026-08-05T08:00:00Z' };
}

function json(body: unknown, status = 200) {
  return { status, contentType: 'application/json', headers: { 'access-control-allow-origin': 'http://127.0.0.1:4173' }, body: JSON.stringify(body) };
}
