import { expect, test } from '@playwright/test';

const sources = {
  epa: 'https://www.epa.gov/biosolids/basic-information-about-sewage-sludge-and-biosolids',
  michiganAnnual: 'https://www.michigan.gov/mdard/-/media/Project/Websites/mdard/documents/environment/rtf/biosolids/biosolids-annual-report-2022.pdf?hash=835344DE72A521B81A5CB4D9D518503C&rev=115337db13924d00b2264561922037bf',
  michiganPfas: 'https://www.michigan.gov/egle/about/Organization/Water-Resources/biosolids/pfas-related',
  michiganPermit: 'https://www.michigan.gov/-/media/Project/Websites/egle/Documents/Programs/WRD/NPDES/General-Permits/MIG960000-biosolids-application-general-permit-2025.pdf?rev=b6f483dc8c624a1ebf78c3fff928bd1b',
};

test('explains the problem, evidence, method, and authorization boundary', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.goto('/about');

  await expect(page).toHaveTitle('About | FieldProof');
  await expect(page.getByRole('heading', { name: 'Why FieldProof exists' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'One placement decision depends on facts kept in different places.' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Land application has real value, and the rules are field-specific.' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'The agent produces a bounded proposal, not a permit.' })).toBeVisible();

  await expect(page.getByRole('heading', { name: '4 million dry metric tons' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '90,239 dry tons' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '173 facilities' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '6% surface · 12% injection' })).toBeVisible();
  await expect(page.getByText(/not a FieldProof savings claim/)).toBeVisible();
  await expect(page.getByText(/does not prove whole-field terrain suitability/)).toBeVisible();

  await expect(page.getByRole('link', { name: /U.S. EPA, 2024 reporting data/ })).toHaveAttribute('href', sources.epa);
  await expect(page.getByRole('link', { name: /Michigan MDARD, 2022 annual report/ })).toHaveAttribute('href', sources.michiganAnnual);
  await expect(page.getByRole('link', { name: /Michigan EGLE, 2024 PFAS results/ })).toHaveAttribute('href', sources.michiganPfas);
  await expect(page.getByRole('link', { name: /Michigan EGLE, General Permit MIG960000/ }).first()).toHaveAttribute('href', sources.michiganPermit);
  const preparedCase = page.getByRole('link', { name: 'Open the prepared case' });
  await expect(preparedCase).toHaveAttribute('href', '/judge-demo');
  await expect(preparedCase).toHaveCSS('color', 'rgb(255, 255, 255)');
});

test('keeps the about explanation readable at 320px', async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 800 });
  await page.goto('/about');

  await expect(page.getByRole('navigation', { name: 'Primary' }).getByRole('link', { name: 'About' })).toHaveAttribute('aria-current', 'page');
  await expect(page.getByRole('heading', { name: 'Why FieldProof exists' })).toBeVisible();
  await expect(page.getByRole('link', { name: 'Open the prepared case' })).toBeVisible();
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  expect(overflow).toBeLessThanOrEqual(0);
});
