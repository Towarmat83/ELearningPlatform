import { test, expect } from '@playwright/test';

test.describe('pages publiques', () => {
  test('page login', async ({ page }) => {
    await page.goto('/login');
    await page.waitForLoadState('networkidle');

    await expect(page).toHaveScreenshot('login.png', { fullPage: true });
  });
});
