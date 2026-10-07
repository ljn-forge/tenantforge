import { defineConfig } from '@playwright/test'
import process from 'node:process'
export default defineConfig({
  testDir: './e2e',
  fullyParallel: false,
  workers: 1,
  timeout: 120000,
  expect: { timeout: 10000 },
  reporter: 'list',
  use: {
    actionTimeout: 10000,
    channel: process.env.TF_E2E_CHANNEL ?? 'chrome',
    baseURL: process.env.TF_E2E_URL ?? 'http://localhost:5178',
    viewport: { width: 1440, height: 1000 },
    trace: 'off',
    screenshot: 'off',
  },
})
