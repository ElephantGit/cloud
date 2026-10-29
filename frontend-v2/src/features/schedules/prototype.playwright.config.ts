/// <reference types="node" />
import path from 'node:path'
import { defineConfig } from '@playwright/test'
import base from '../../../playwright.config'

// Dedicated ports leave other feature worktrees' dev/e2e servers untouched.
const root = path.resolve(import.meta.dirname, '../../..')
export default defineConfig({
  ...base,
  testDir: root,
  testMatch: ['**/e2e/shell.spec.ts', '**/schedules/prototype.e2e.ts'],
  outputDir: path.join(root, 'test-results'),
  use: { ...base.use, baseURL: 'http://127.0.0.1:18870' },
  webServer: [
    { command: 'go run .', cwd: path.join(root, '../v2-server'), url: 'http://127.0.0.1:18871/healthz', env: { V2_ADDR: '127.0.0.1:18871', V2_DATABASE_URL: process.env.E2E_DATABASE_URL ?? 'postgres://ora:ora-local@127.0.0.1:55432/ora_v2_e2e?sslmode=disable' }, reuseExistingServer: false, timeout: 120_000 },
    { command: 'npx vite --port 18870 --strictPort --host 127.0.0.1', cwd: root, url: 'http://127.0.0.1:18870', env: { V2_SERVER: 'http://127.0.0.1:18871' }, reuseExistingServer: false },
  ],
})
