import { expect, test, type Page } from '@playwright/test';

const queuedRun = {
  id: 'd7da7a6a-1a7c-40db-a1ef-30e2d3446063',
  kind: 'MIREYE_READINESS',
  status: 'QUEUED',
  nextStep: 1,
  createdAt: '2026-08-04T12:00:00Z',
  updatedAt: '2026-08-04T12:00:00Z',
  steps: [
    { id: '8b01cc29-69d1-45c0-8e9b-1a1a76e5dba0', position: 1, toolName: 'mireye.meta.fields', status: 'PENDING', attemptCount: 0 },
    { id: 'c78a56ae-80d2-4e69-ae5d-ad31078fda3d', position: 2, toolName: 'mireye.meta.plans', status: 'PENDING', attemptCount: 0 },
    { id: 'ae4b775c-e243-4a68-970b-cee08a86fd71', position: 3, toolName: 'mireye.users.me.usage', status: 'PENDING', attemptCount: 0 },
  ],
  toolCalls: [],
  dataGaps: [],
};

const readyRun = {
  ...queuedRun,
  status: 'SUCCEEDED',
  nextStep: 4,
  startedAt: '2026-08-04T12:00:01Z',
  completedAt: '2026-08-04T12:00:04Z',
  updatedAt: '2026-08-04T12:00:04Z',
  steps: queuedRun.steps.map((step) => ({
    ...step,
    status: 'SUCCEEDED',
    attemptCount: 1,
    summary: step.position === 1
      ? { fieldCount: 292, presetCount: 15, catalogVersion: '0.14.0' }
      : step.position === 2
        ? { planCount: 5, hasCreditCosts: true }
        : { planName: 'Build', creditsRemaining: 24684, resetsAt: '2026-09-01T00:00:00Z' },
  })),
  toolCalls: queuedRun.steps.map((step, index) => ({
    id: `2ef58ba1-e970-442a-a455-71e65c34fe4${index}`,
    stepId: step.id,
    attempt: 1,
    status: 'SUCCEEDED',
    method: 'GET',
    path: ['/v1/meta/fields', '/v1/meta/plans', '/v1/users/me/usage'][index],
    requestHash: 'a'.repeat(64),
    responseHash: 'b'.repeat(64),
    requestId: `req_fixture_${index + 1}`,
    sourceUrl: `https://api.mireye.com${['/v1/meta/fields', '/v1/meta/plans', '/v1/users/me/usage'][index]}`,
    httpStatus: 200,
    durationMs: 180 + index,
    creditCost: 0,
    fetchedAt: `2026-08-04T12:00:0${index + 1}Z`,
  })),
};

test('checks data access and keeps technical evidence available on demand', async ({ page }) => {
  await mockAPI(page);
  await page.goto('/data-access');

  await expect(page.getByRole('heading', { name: 'Not checked yet' })).toBeVisible();
  await page.getByRole('button', { name: 'Check access' }).click();
  await expect(page.getByText('Waiting', { exact: true }).first()).toBeVisible();
  await expect(page.getByText('3 of 3 verified')).toBeVisible();
  await expect(page.getByRole('link', { name: 'Mireye property data source' })).toBeVisible();
  await expect(page.getByText('req_fixture_1')).not.toBeVisible();
  await page.getByText('Technical details', { exact: true }).first().click();
  await expect(page.getByText('req_fixture_1')).toBeVisible();
  await expect(page.locator('details[open] dt').getByText('Credits used', { exact: true })).toBeVisible();
});

test('keeps the evidence workflow inside a 320px viewport', async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 800 });
  await mockAPI(page, readyRun);
  await page.goto('/data-access');

  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  expect(overflow).toBeLessThanOrEqual(0);
  await expect(page.getByRole('heading', { name: 'Check data access' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Checks' })).toBeVisible();
});

async function mockAPI(page: Page, initialRun: typeof readyRun | null = null) {
  let currentRun = initialRun;
  await page.route('http://localhost:8080/api/v1/readiness-runs/latest', async (route) => {
    if (currentRun) {
      await route.fulfill(json(currentRun));
    } else {
      await route.fulfill(json({ status: 404, detail: 'no readiness run exists' }, 404));
    }
  });
  await page.route('http://localhost:8080/api/v1/readiness-runs', async (route) => {
    currentRun = queuedRun as typeof readyRun;
    await route.fulfill(json({ run: currentRun, created: true }, 201));
  });
  await page.route(`http://localhost:8080/api/v1/readiness-runs/${queuedRun.id}`, async (route) => {
    currentRun = readyRun;
    await route.fulfill(json(currentRun));
  });
}

function json(body: unknown, status = 200) {
  return {
    status,
    contentType: 'application/json',
    headers: { 'access-control-allow-origin': 'http://127.0.0.1:4173' },
    body: JSON.stringify(body),
  };
}
