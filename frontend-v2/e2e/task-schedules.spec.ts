/// <reference lib="dom" />
import { expect, test } from '@playwright/test'

test('unavailable target kinds stay disabled and no sample project is presented as executable', async ({ page }) => {
  await page.goto('/schedules')
  await page.getByRole('button', { name: '开发者登录' }).click()
  await page.locator('header').getByRole('button', { name: '新建任务', exact: true }).click()
  const dialog = page.getByRole('dialog')
  await expect(dialog.getByRole('button', { name: /运行工作流/ })).toBeDisabled()
  await expect(dialog.getByRole('button', { name: /已有任务/ })).toBeDisabled()
  // The assembled runtime exposes the credential-free Fixture model and seeded Demo project,
  // while CI without Node assets keeps the Agent executor list empty.
  const modelSelect = dialog.getByRole('combobox', { name: '模型', exact: true })
  const executorSelect = dialog.getByRole('combobox', { name: '执行器', exact: true })
  await expect(modelSelect).toBeEnabled()
  await expect(executorSelect).toBeDisabled()
  await expect(dialog.getByRole('option', { name: 'Fixture', exact: true })).toHaveCount(1)
  await expect(dialog.getByRole('option', { name: 'Ora 官网', exact: true })).toHaveCount(0)
  await expect(dialog.getByRole('option', { name: '产品研发', exact: true })).toHaveCount(0)
  await expect(page.getByRole('button', { name: '模拟运行', exact: true })).toHaveCount(0)
  await dialog.getByRole('combobox', { name: '关联项目', exact: true }).selectOption({ label: 'Demo' })
  await modelSelect.selectOption('fixture')
  await expect(executorSelect).toContainText('暂无匹配的执行器')
  await dialog.getByRole('textbox', { name: '任务名称' }).fill('未保存的输入')
  await page.keyboard.press('Escape')
  await expect(dialog.getByText('放弃未保存的修改？')).toBeVisible()
  await dialog.getByRole('button', { name: '继续编辑' }).click()
  await page.setViewportSize({ width: 390, height: 844 })
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await page.screenshot({ path: test.info().outputPath('resources-unavailable-mobile.png'), fullPage: true })
})
