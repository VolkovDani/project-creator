import { defineConfig } from '@playwright/test';
export default defineConfig({
  testDir: './e2e',
  testMatch: 'screenshots.spec.ts',
  workers: 1,
  use: {
    baseURL: 'http://127.0.0.1:3101',
    viewport: { width: 1920, height: 1080 },
    launchOptions: { args: ['--no-sandbox'] },
  },
  webServer: {
    command: 'bun run start -- -p 3101',
    url: 'http://127.0.0.1:3101',
    reuseExistingServer: false,
    timeout: 120000,
  },
});
