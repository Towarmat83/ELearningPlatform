import { defineConfig, devices } from '@playwright/test';
import { execSync } from 'child_process';

const baseURL = process.env.BASE_URL ?? 'http://localhost:4321';

// En CI ADMIN_PASSWORD est fourni via l'env du workflow.
// En local, on le récupère depuis le secret Kubernetes si non défini.
if (!process.env.ADMIN_PASSWORD && !process.env.CI) {
  try {
    process.env.ADMIN_PASSWORD = execSync(
      "kubectl get secret elearning-secrets -o jsonpath='{.data.ADMIN_PASSWORD}' | base64 -d",
      { shell: '/bin/bash', stdio: 'pipe' },
    ).toString().trim();
  } catch {
    // kubectl absent ou cluster non démarré — auth.setup.ts utilisera 'admin'
  }
}

export default defineConfig({
  testDir: './e2e',
  fullyParallel: false,
  retries: process.env.CI ? 1 : 0,
  workers: 1,
  reporter: process.env.CI
    ? [['html', { open: 'never', outputFolder: 'playwright-report' }], ['github']]
    : [['html', { open: 'never' }], ['list']],

  use: {
    baseURL,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },

  expect: {
    toHaveScreenshot: {
      maxDiffPixelRatio: 0.02,
      animations: 'disabled',
    },
  },

  projects: [
    { name: 'setup-admin', testMatch: /auth\.setup\.ts/ },
    {
      name: 'chromium',
      use: {
        ...devices['Desktop Chrome'],
        viewport: { width: 1280, height: 800 },
        storageState: 'e2e/.auth/admin.json',
        locale: 'fr-FR',
      },
      dependencies: ['setup-admin'],
      testMatch: /.*\.spec\.ts/,
    },
    {
      name: 'chromium-public',
      use: {
        ...devices['Desktop Chrome'],
        viewport: { width: 1280, height: 800 },
        locale: 'fr-FR',
      },
      testMatch: /public\.spec\.ts/,
    },
  ],

  webServer: {
    command: 'npm run dev',
    url: baseURL,
    reuseExistingServer: true,
    timeout: 120_000,
  },
});
