import { expect, test } from '@playwright/test'

test('workflow runs: start a published workflow from chat and follow it to the result', async ({ page }) => {
  // The demo engine takes 1.5 s a node, so the sample needs about ten seconds to finish.
  test.setTimeout(90_000)
  // v2-server has no project or execution runtime wired yet, so the chat page is given one project
  // and executor to hang the run on; the run itself plays on the in-browser demo engine.
  await page.route('**/api/v2/tenants/*/projects?*', (route) =>
    route.fulfill({ json: { items: [{ id: '00000000-0000-4000-8000-0000000000c1', tenantId: '00000000-0000-4000-8000-0000000000aa', ownerUserId: '00000000-0000-4000-8000-0000000000bb', name: 'ora-space/cloud', repositoryUrl: 'https://github.com/ora-space/cloud', defaultBranch: 'main', commitId: '0'.repeat(40), createdAt: '2026-10-30T09:00:00Z' }], nextCursor: '' } }),
  )
  await page.route('**/api/v2/tenants/*/chat/executors', (route) =>
    route.fulfill({ json: { items: [{ id: 'oc-fixture', name: 'OpenCode · Fixture', agentName: 'OpenCode', modelName: 'Fixture' }] } }),
  )
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

  // Start asks for the run's input, then the Theater plays the nodes one after another to the result act.
  await pane.getByRole('button', { name: '启动' }).click()
  const start = page.getByRole('dialog')
  await start.getByLabel('初始 Prompt').fill('审查 cloud#210 的改动')
  await start.getByRole('button', { name: '启动' }).click()
  await expect(pane.getByRole('button', { name: '舞台', exact: true })).toHaveAttribute('aria-pressed', 'true')
  await expect(cloud.getByRole('button', { name: '工作流运行 审查 e2e（运行中）' })).toBeVisible()
  await expect(cloud.getByRole('button', { name: '工作流运行 审查 e2e（成功）' })).toBeVisible({ timeout: 60_000 })
  await expect(pane.getByText('运行完成')).toBeVisible()

  // Overview shows the whole graph; a node opens its act, which keeps what the Agent was sent and answered.
  await pane.getByRole('button', { name: '全图', exact: true }).click()
  await pane.getByLabel('工作流全图俯瞰').getByText('Agent：审查改动', { exact: true }).click()
  await pane.getByRole('button', { name: '查看节点会话' }).click()
  await expect(pane.getByRole('heading', { name: 'Mock 结论' })).toBeVisible()

  // A session takes the main area back; the run stays in the list.
  await page.getByRole('complementary', { name: '聊天记录' }).getByRole('button', { name: '工具调用：读、搜、改、执行', exact: true }).click()
  await expect(pane).toBeHidden()
  await expect(cloud.getByRole('button', { name: '工作流运行 审查 e2e（成功）' })).toBeVisible()
})
