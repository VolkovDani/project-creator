import { defineConfig } from '@playwright/test';

// Deterministic browser regressions: every API/model response is intercepted.
export default defineConfig({
  testDir: './e2e',
  testMatch: ['ux.spec.ts', 'analysis.spec.ts', 'analysis-live.spec.ts', 'rag.spec.ts'],
  timeout: 30000,
  workers: 1,
  use: {
    baseURL: 'http://127.0.0.1:3101',
    launchOptions: {
      args: ['--no-sandbox'],
      ...(process.env.E2E_CHROMIUM_PATH ? { executablePath: process.env.E2E_CHROMIUM_PATH } : {}),
    },
  },
  projects: [
    { name: 'desktop', use: { viewport: { width: 1920, height: 1080 } } },
    { name: 'narrow', testMatch: 'ux.spec.ts', use: { viewport: { width: 390, height: 844 } } },
  ],
  webServer: {
    command: 'npm run start -- -p 3101',
    url: 'http://127.0.0.1:3101',
    reuseExistingServer: false,
    timeout: 120000,
  },
});
