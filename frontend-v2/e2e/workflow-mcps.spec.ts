import { expect, test } from '@playwright/test'
import { signIn } from './login.ts'

test('Agent MCP choices reuse Settings installs and preserve bindings after removal', async ({ page }, info) => {
  test.setTimeout(120_000)
  await page.setViewportSize({ width: 1440, height: 1000 })
  await page.goto('/settings/plugins')
  await signIn(page)
  const plugin = page.getByRole('article', { name: 'Tavily', exact: true })
  await plugin.getByRole('button', { name: '安装', exact: true }).click()
  await expect(plugin.getByText('已安装', { exact: true })).toBeVisible({ timeout: 15_000 })
  const tid = (await (await page.request.get('/api/v1/me/tenants')).json()).items[0].id
  const base = `/api/v2/tenants/${tid}/workflows`
  const name = `MCP 设置联动-${Date.now()}`
  const response = await page.request.post(base, { data: { name, graph: null } })
  expect(response.status()).toBe(201)
  const wid = (await response.json()).workflow.id
  const open = async () => {
    await page.goto('/workflows')
    await page.getByRole('complementary', { name: '工作流列表' }).getByRole('button', { name, exact: true }).click()
  }
  await open()
  await page.getByRole('toolbar', { name: '添加工作流节点' }).getByRole('button', { name: 'Agent', exact: true }).click()
  await page.getByRole('button', { name: '添加 MCP', exact: true }).click()
  const choice = page.getByRole('option', { name: 'Tavily（演示） official/ora-space.tavily-search', exact: true })
  await expect(choice).toBeVisible()
  await page.screenshot({ path: info.outputPath('workflow-mcp-picker.png'), animations: 'disabled' })
  await choice.click()
  const node = async () => {
    const snapshot = (await (await page.request.get(`${base}/${wid}/draft`)).json()).snapshot
    return JSON.parse(snapshot.graph).nodes.find((n: { data: { kind: string } }) => n.data.kind === 'agent')
  }
  await expect.poll(async () => (await node())?.data.agentConfig.mcps).toEqual([{ mcpId: 'official/ora-space.tavily-search', enabled: true }])
  const agentId = (await node()).id
  await page.reload()
  await page.getByRole('button', { name, exact: true }).click()
  await page.locator(`.react-flow__node[data-id="${agentId}"]`).click()
  await expect(page.getByRole('switch', { name: '启用或禁用 Tavily（演示）', exact: true })).toBeChecked()
  await expect(page.getByText('可选择设置中已安装的 MCP；演示项仅用于节点配置，当前 v2 不支持 MCP 执行。')).toBeVisible()
  await page.goto('/settings/plugins')
  await plugin.getByRole('button', { name: '移除', exact: true }).click()
  await page.getByRole('alertdialog').getByRole('button', { name: '移除', exact: true }).click()
  await expect(plugin.getByText('未安装', { exact: true })).toBeVisible()
  await open()
  await page.locator(`.react-flow__node[data-id="${agentId}"]`).click()
  await expect(page.getByRole('button', { name: '添加 MCP', exact: true })).toBeDisabled()
  await expect(page.getByText('插件未安装或不可用', { exact: true })).toBeVisible()
  expect((await node()).data.agentConfig.mcps).toEqual([{ mcpId: 'official/ora-space.tavily-search', enabled: true }])
})
