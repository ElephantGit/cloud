import { expect, test } from '@playwright/test'

test('workflow runs: start a published workflow from chat and follow it to the result', async ({ page }) => {
  // The demo engine takes 1.5 s a node, so the sample needs about ten seconds to finish.
  test.setTimeout(90_000)
  await page.goto('/chat')
  await page.getByRole('button', { name: '开发者登录' }).click()
  const cloud = page.getByRole('complementary', { name: '聊天记录' }).getByRole('region').filter({ hasText: 'ora-space/cloud' })

  // A project's + offers the published workflows; the run opens pending in the main area.
  await cloud.getByRole('button', { name: '在 ora-space/cloud 新建对话' }).click()
  await page.getByRole('menuitem', { name: '运行工作流' }).hover()
  await page.getByRole('menuitem', { name: /示例：代码审查/ }).click()
  const dialog = page.getByRole('dialog', { name: '运行工作流' })
  await dialog.getByLabel('运行名称').fill('审查 e2e')
  await dialog.getByRole('button', { name: '创建运行' }).click()
  const pane = page.getByRole('region', { name: '工作流运行' })
  await expect(pane.getByRole('heading', { name: '审查 e2e' })).toBeVisible()
  await expect(pane.getByText('演示运行')).toBeVisible()
  await expect(cloud.getByRole('button', { name: '工作流运行 审查 e2e（未运行）' })).toBeVisible()

  // Start asks for the run's input, then the nodes run one after another to the Output.
  await pane.getByRole('button', { name: '启动' }).click()
  const start = page.getByRole('dialog')
  await start.getByLabel('初始 Prompt').fill('审查 cloud#210 的改动')
  await start.getByRole('button', { name: '启动' }).click()
  await expect(cloud.getByRole('button', { name: '工作流运行 审查 e2e（运行中）' })).toBeVisible()
  await expect(cloud.getByRole('button', { name: '工作流运行 审查 e2e（成功）' })).toBeVisible({ timeout: 60_000 })
  await expect(pane.getByRole('region', { name: '运行结果' })).toContainText('运行结果 · 成功')

  // An Agent node keeps what it was sent and what it answered.
  await pane.getByLabel('工作流全图俯瞰').getByText('Agent：审查改动', { exact: true }).click()
  await expect(pane.getByRole('complementary', { name: '节点详情' }).getByLabel('Agent 回复').first()).toBeVisible()

  // A session takes the main area back; the run stays in the list.
  await cloud.getByRole('button', { name: '梳理 v2 接口缺口', exact: true }).click()
  await expect(pane).toBeHidden()
  await expect(cloud.getByRole('button', { name: '工作流运行 审查 e2e（成功）' })).toBeVisible()
})
