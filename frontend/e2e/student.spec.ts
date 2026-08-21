import { test, expect } from '@playwright/test';
import { waitForPageReady } from './helpers';

test.describe('pages étudiant', () => {
  test('catalogue des cours', async ({ page }) => {
    await page.goto('/courses');
    await waitForPageReady(page);

    await expect(page).toHaveScreenshot('catalogue.png', { fullPage: true });
  });

  test('page cours', async ({ page }) => {
    await page.goto('/courses/linux-intro');
    await waitForPageReady(page);

    await expect(page).toHaveScreenshot('course-detail.png', { fullPage: true });
  });

  test('mes cours', async ({ page }) => {
    await page.goto('/my-courses');
    await waitForPageReady(page);

    await expect(page).toHaveScreenshot('my-courses.png', { fullPage: true });
  });

  test('classement', async ({ page }) => {
    await page.goto('/leaderboard');
    await waitForPageReady(page);

    // mask scores and dates that change between runs
    await expect(page).toHaveScreenshot('leaderboard.png', {
      fullPage: true,
      mask: [page.locator('td + td'), page.locator('.xp-value')],
    });
  });

  test('mes XP', async ({ page }) => {
    await page.goto('/my-xp');
    await waitForPageReady(page);

    await expect(page).toHaveScreenshot('my-xp.png', {
      fullPage: true,
      mask: [page.locator('.xp-value'), page.locator('time')],
    });
  });

  test('mes parcours', async ({ page }) => {
    await page.goto('/my-paths');
    await waitForPageReady(page);

    await expect(page).toHaveScreenshot('my-paths.png', { fullPage: true });
  });
});
