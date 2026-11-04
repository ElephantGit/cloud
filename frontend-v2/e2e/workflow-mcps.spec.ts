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
    await expect(page.getByLabel('工作流名称', { exact: true })).toHaveValue(name)
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
  await expect.poll(async () => (await node())?.data.agentConfig?.mcps).toEqual([{ mcpId: 'official/ora-space.tavily-search', enabled: true }])
  const agentId = (await node()).id
  await page.reload()
  await page.getByRole('button', { name, exact: true }).click()
  await page.locator(`.react-flow__node[data-id="${agentId}"]`).click()
  await expect(page.getByRole('switch', { name: '启用或禁用 Tavily（演示）', exact: true })).toBeChecked()
  await expect(page.getByText('可选择设置中已安装的 MCP；启用后 Agent 照常运行，MCP 不参与执行，结果标记为“未应用”。')).toBeVisible()
  await page.screenshot({ path: info.outputPath('workflow-mcp-enabled.png'), animations: 'disabled' })
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

test('an Agent with enabled MCP completes and reports mcp as unapplied', async ({ page }) => {
  test.setTimeout(180_000)
  await page.goto('/workflows')
  await signIn(page)
  const tid = (await (await page.request.get('/api/v1/me/tenants')).json()).items[0].id
  const base = `/api/v2/tenants/${tid}`
  const executors = (await (await page.request.get(`${base}/chat/executors`)).json()).items
  test.skip(executors.length === 0, 'prepared Node assets are required for Agent execution')
  const executor = executors[0]
  const graph = JSON.stringify({
    nodes: [
      { id: 'start', data: { kind: 'start' } },
      { id: 'agent', data: { kind: 'agent', agentConfig: {
        executor: { agentCli: executor.agentId, modelId: executor.modelId },
        prompt: 'Reply with a short greeting.',
        mcps: [{ mcpId: 'official/ora-space.tavily-search', enabled: true }],
      } } },
      { id: 'out', data: { kind: 'output', outputs: [{ name: 'answer', variableSelector: ['agent', 'output'] }] } },
    ],
    edges: [{ source: 'start', target: 'agent' }, { source: 'agent', target: 'out' }],
  })
  const created = await page.request.post(`${base}/workflows`, { data: { name: `MCP unapplied ${Date.now()}`, graph } })
  expect(created.status()).toBe(201)
  const wid = (await created.json()).workflow.id
  expect((await page.request.post(`${base}/workflows/${wid}/publish`, { data: { version: 'v1' } })).ok()).toBe(true)
  const project = (await (await page.request.get(`${base}/projects?limit=100`)).json()).items[0]
  const run = await page.request.post(`${base}/workflows/${wid}/runs`, { data: { projectId: project.id, name: 'MCP ignored', input: 'hello' } })
  expect(run.status()).toBe(201)
  const rid = (await run.json()).run.id
  expect((await page.request.post(`${base}/workflow-runs/${rid}/start`)).ok()).toBe(true)
  const detail = async () => (await page.request.get(`${base}/workflow-runs/${rid}`)).json()
  await expect.poll(async () => (await detail()).run.status, { timeout: 120_000 }).toBe('succeeded')
  const node = (await detail()).nodes.find((n: { nodeId: string }) => n.nodeId === 'agent')
  expect(node.sessionId).toBeTruthy()
  expect(JSON.parse(node.payload).unapplied).toContain('mcp')
})
