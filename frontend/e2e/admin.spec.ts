import { test, expect } from '@playwright/test';
import { waitForPageReady } from './helpers';

test.describe('pages admin', () => {
  test('tableau de bord admin', async ({ page }) => {
    await page.goto('/admin');
    await waitForPageReady(page);

    await expect(page).toHaveScreenshot('admin-dashboard.png', { fullPage: true });
  });

  test('gestion utilisateurs', async ({ page }) => {
    await page.goto('/admin/users');
    await waitForPageReady(page);

    await expect(page).toHaveScreenshot('admin-users.png', {
      fullPage: true,
      mask: [
        page.locator('td:has-text("@")'),
        page.locator('td').filter({ hasText: /\d{4}-\d{2}-\d{2}/ }),
      ],
    });
  });

  test('gestion cours', async ({ page }) => {
    await page.goto('/admin/courses');
    await waitForPageReady(page);

    await expect(page).toHaveScreenshot('admin-courses.png', { fullPage: true });
  });

  test('exports CSV', async ({ page }) => {
    await page.goto('/admin/exports');
    await waitForPageReady(page);

    // wait for categories to load
    await page.locator('#export-body').waitFor({ state: 'visible', timeout: 8_000 });

    await expect(page).toHaveScreenshot('admin-exports.png', { fullPage: true });
  });

  test('exports CSV — aperçu utilisateurs', async ({ page }) => {
    await page.goto('/admin/exports');
    await page.locator('#export-body').waitFor({ state: 'visible', timeout: 8_000 });

    await page.locator('button', { hasText: 'Utilisateurs' }).click();
    await page.locator('#preview-table-wrap').waitFor({ state: 'visible', timeout: 8_000 });

    await expect(page).toHaveScreenshot('admin-exports-preview.png', {
      fullPage: true,
      mask: [
        page.locator('#preview-body td'),
      ],
    });
  });

  test('parcours admin', async ({ page }) => {
    await page.goto('/admin/paths');
    await waitForPageReady(page);

    await expect(page).toHaveScreenshot('admin-paths.png', { fullPage: true });
  });
});
