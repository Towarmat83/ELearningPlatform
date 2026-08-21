import { test as setup, expect } from '@playwright/test';

const adminEmail    = process.env.ADMIN_EMAIL    ?? 'admin@pupitre.local';
const adminPassword = process.env.ADMIN_PASSWORD ?? 'admin';

setup('authenticate as admin', async ({ page }) => {
  await page.goto('/login');
  await page.waitForLoadState('networkidle');

  await page.fill('#email', adminEmail);
  await page.fill('#password', adminPassword);
  await page.click('#login-btn');

  // Admin is redirected to /admin after login
  await page.waitForURL('**/admin', { timeout: 10_000 });
  await expect(page).toHaveURL(/\/admin/);

  await page.context().storageState({ path: 'e2e/.auth/admin.json' });
});
