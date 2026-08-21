import { expect, test, type Page } from '@playwright/test';

const origins = ['/', '/judge-demo', '/coordination', '/coordination/workflow/missing-record', '/data-access', '/not-a-route'];
const destinations = [
  { label: 'New case', path: '/' },
  { label: 'Prepared case', path: '/judge-demo' },
  { label: 'Coordination', path: '/coordination' },
];

test('keeps every global workspace distinct and reachable from every route', async ({ page }) => {
  test.setTimeout(45_000);
  await page.route('http://localhost:8080/api/v1/**', async (route) => route.fulfill({
    status: 503,
    contentType: 'application/json',
    body: JSON.stringify({ detail: 'Navigation test fixture' }),
  }));

  for (const origin of origins) {
    await openRoute(page, origin);
    const primary = page.getByRole('navigation', { name: 'Primary' });
    const hrefs = await primary.locator('a').evaluateAll((links) => links.map((link) => link.getAttribute('href')));
    expect(hrefs).toEqual(['/', '/judge-demo', '/coordination']);
    expect(new Set(hrefs).size).toBe(hrefs.length);

    for (const destination of destinations) {
      await openRoute(page, origin);
      await page.getByRole('navigation', { name: 'Primary' }).getByRole('link', { name: destination.label }).click();
      await expect(page).toHaveURL(destination.path);
    }

    await openRoute(page, origin);
    await page.getByText('Setup', { exact: true }).click();
    await page.getByRole('link', { name: 'Data access' }).click();
    await expect(page).toHaveURL('/data-access');
  }
});

test('shows a real not-found page for an unknown route', async ({ page }) => {
  await page.goto('/not-a-route');

  await expect(page).toHaveTitle('Page Not Found | FieldProof');
  await expect(page.getByRole('heading', { name: 'Page not found' })).toBeVisible();
  await expect(page.getByRole('link', { name: 'Start a new case' })).toHaveAttribute('href', '/');
});

async function openRoute(page: Page, path: string) {
  await page.goto(path);
  await expect(page.getByRole('heading', { name: 'Loading workspace' })).toHaveCount(0);
  await expect(page.locator('.topbar')).toBeVisible();
}
