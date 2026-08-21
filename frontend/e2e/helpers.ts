import { Page, Locator } from '@playwright/test';

/** Waits for the loading spinner to disappear and the page content to stabilise. */
export async function waitForPageReady(page: Page): Promise<void> {
  // Timeouts explicites pour ne pas consommer tout le budget du test (30s par défaut).
  // Les deux attentes sont cumulables donc on les plafonne à ~8s chacune.
  await page.waitForLoadState('networkidle', { timeout: 8_000 }).catch(() => {});
  const spinner = page.locator('.spinner');
  if (await spinner.count() > 0) {
    await spinner.first().waitFor({ state: 'hidden', timeout: 8_000 }).catch(() => {});
  }
}

/**
 * Returns locators for elements whose content varies between runs
 * (timestamps, UUIDs, XP totals) so they can be masked in screenshots.
 */
export function dynamicLocators(page: Page): Locator[] {
  return [
    page.locator('time'),
    page.locator('[data-testid="dynamic"]'),
    page.locator('td:has-text("-")').filter({ hasText: /\d{4}-\d{2}-\d{2}/ }),
  ];
}
