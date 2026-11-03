import { expect, test } from '@playwright/test'

test('reasoning keeps its estimate when output settles and the page reloads', async ({ page }, testInfo) => {
  await page.goto('/e2e/fixtures/thought-timing.html')
  await expect(page.getByText('思考中', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: '显示回答' }).click()
  await expect(page.getByText('思考了约 5 秒', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: '完成本轮' }).click()
  await expect(page.getByText('思考了约 5 秒', { exact: true })).toBeVisible()
  await page.evaluate(() => localStorage.clear())
  await page.reload()
  await expect(page.getByText('思考了约 5 秒', { exact: true })).toBeVisible()
  await expect(page.getByText('思考了 0.0 秒', { exact: true })).toHaveCount(0)
  await page.screenshot({ path: testInfo.outputPath('thought-duration-reloaded.png'), fullPage: true })
})

test('settled-only reasoning has no fabricated duration', async ({ page }) => {
  await page.goto('/e2e/fixtures/thought-timing.html?legacy=1')
  await expect(page.getByText('思考', { exact: true })).toBeVisible()
  await expect(page.getByText(/思考了/)).toHaveCount(0)
})
