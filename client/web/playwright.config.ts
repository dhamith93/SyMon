import { defineConfig } from '@playwright/test';

// Smoke tests against a running dashboard with at least one agent
// reporting, like `docker compose up`. SYMON_E2E_URL picks the dashboard.
export default defineConfig({
  testDir: 'e2e',
  timeout: 30_000,
  use: {
    baseURL: process.env.SYMON_E2E_URL ?? 'http://localhost:8080',
    viewport: { width: 1400, height: 900 },
  },
});
