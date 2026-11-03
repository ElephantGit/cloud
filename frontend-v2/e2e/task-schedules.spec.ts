/// <reference lib="dom" />
import { expect, test } from '@playwright/test'
import { signIn } from './login.ts'

const agentsAvailable = !!process.env.V2_NODE_ASSETS_DIR

test('workflows are offered, the unwired task target stays disabled, real Fixture model is selectable, and no sample project is presented', async ({ page }) => {
  await page.goto('/schedules')
  await signIn(page)
  await page.locator('header').getByRole('button', { name: '新建任务', exact: true }).click()
  const dialog = page.getByRole('dialog')
  await expect(dialog.getByRole('button', { name: /运行工作流/ })).toBeEnabled()
  await expect(dialog.getByRole('button', { name: /已有任务/ })).toBeDisabled()
  // The assembled runtime exposes the credential-free Fixture model and seeded Demo project,
  // while CI without Node assets keeps the Agent list empty.
  const modelSelect = dialog.getByRole('combobox', { name: '模型', exact: true })
  const agentSelect = dialog.getByRole('combobox', { name: 'Agent', exact: true })
  await expect(modelSelect).toBeEnabled()
  await expect(agentSelect).toBeDisabled()
  await expect(dialog.getByRole('option', { name: 'Fixture', exact: true })).toHaveCount(1)
  await expect(dialog.getByRole('option', { name: 'Ora 官网', exact: true })).toHaveCount(0)
  await expect(dialog.getByRole('option', { name: '产品研发', exact: true })).toHaveCount(0)
  await expect(page.getByRole('button', { name: '模拟运行', exact: true })).toHaveCount(0)
  await dialog.getByRole('combobox', { name: '关联项目', exact: true }).selectOption({ label: 'Demo' })
  await expect(agentSelect).toBeEnabled()
  await expect(modelSelect).toBeDisabled()
  if (agentsAvailable) {
    await expect(agentSelect).toContainText('OpenCode')
    await agentSelect.selectOption({ label: 'OpenCode' })
    await expect(modelSelect).toBeEnabled()
    await expect(modelSelect).toContainText('Fixture')
  } else {
    await expect(agentSelect).toContainText('暂无可用 Agent')
  }
  await dialog.getByRole('textbox', { name: '任务名称' }).fill('未保存的输入')
  await page.keyboard.press('Escape')
  await expect(dialog.getByText('放弃未保存的修改？')).toBeVisible()
  await dialog.getByRole('button', { name: '继续编辑' }).click()
  await page.setViewportSize({ width: 390, height: 844 })
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await page.screenshot({ path: test.info().outputPath('schedule-form-resources-mobile.png'), fullPage: true })
})
