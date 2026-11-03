import { expect, type Page, test } from '@playwright/test'
import { signIn } from './login.ts'

const projects = (page: Page) => page.getByRole('complementary', { name: '聊天记录' }).getByRole('region')

// With prepared Node assets (`task v2:node:prepare`, then V2_NODE_ASSETS_DIR) v2-server runs real
// Agents on the fixture model; without them, as in CI, it can run none.
const realAgents = !!process.env.V2_NODE_ASSETS_DIR

test('chat: lists only the backend\'s projects, says it cannot run Agents yet, resize the column', async ({ page }) => {
  test.skip(realAgents, 'the backend can run Agents')
  await page.goto('/chat')
  await signIn(page)
  await expect(page.getByRole('heading', { name: '今天想构建什么？' })).toBeVisible()

  // The runtime seeds a real demo project. CI has no Node assets, so the executor list remains
  // empty and a new Agent conversation stays explicitly unavailable.
  await expect(page.getByRole('status').filter({ hasText: /后端还不能运行 Agent/ })).toBeVisible()
  // Only what v2-server returns: no sample project, and the demo project has no conversations yet.
  // Other specs may leave projects of their own in a reused database, so only these two are checked.
  const labels = await projects(page).evaluateAll((els) => els.map((e) => e.getAttribute('aria-label')))
  expect(labels).toContain('项目 Demo')
  expect(labels).not.toContain('项目 示例')
  await expect(projects(page).filter({ hasText: 'Demo' }).getByText('还没有会话')).toBeVisible()

  // A send is refused up front rather than failing on the backend.
  await page.getByRole('textbox', { name: '消息' }).fill('你好')
  await page.keyboard.press('Enter')
  await expect(page.getByRole('heading', { level: 2, name: '新对话' })).toBeVisible()

  // The column's edge resizes it; dragged onto the icon rail, the column folds away.
  const column = page.getByRole('complementary', { name: '聊天记录' })
  const edge = (await page.getByRole('separator', { name: '调整对话列表宽度' }).boundingBox())!
  await page.mouse.move(edge.x + edge.width / 2, edge.y + 200)
  await page.mouse.down()
  await page.mouse.move(edge.x + 100, edge.y + 200, { steps: 5 })
  await expect.poll(async () => Math.round((await column.boundingBox())!.width)).toBeGreaterThan(370)
  await page.mouse.move(10, edge.y + 200, { steps: 10 })
  await page.mouse.up()
  await expect(column).toBeHidden()
  await page.getByRole('button', { name: '展开对话列表' }).click()
  await expect(column).toBeVisible()
})

/**
 * Gives the page two projects and an executor, so it can reorder them and save the order. Only
 * these two lists are stand-ins: the order itself is saved to and read back from v2-server.
 */
async function twoProjects(page: Page) {
  const project = (id: string, name: string) => ({
    id, tenantId: '00000000-0000-4000-8000-0000000000aa', ownerUserId: '00000000-0000-4000-8000-0000000000bb', name,
    repositoryUrl: `https://github.com/ora-space/${name}`, defaultBranch: 'main', commitId: '0'.repeat(40), createdAt: '2026-10-31T09:00:00Z',
  })
  await page.route('**/api/v2/tenants/*/projects?*', (route) =>
    route.fulfill({ json: { items: [project('00000000-0000-4000-8000-0000000000e1', 'cloud'), project('00000000-0000-4000-8000-0000000000e2', 'desktop')], nextCursor: '' } }),
  )
  await page.route('**/api/v2/tenants/*/chat/executors', (route) =>
    route.fulfill({ json: { items: [{ id: 'oc-fixture', name: 'OpenCode · Fixture', agentName: 'OpenCode', modelName: 'Fixture' }] } }),
  )
  await page.goto('/chat')
  await signIn(page)
  await expect(page.getByRole('heading', { name: '今天想构建什么？' })).toBeVisible()
}

const order = (page: Page) => projects(page).evaluateAll((els) => els.map((e) => e.getAttribute('aria-label')))

test('chat: a dragged project order is kept by v2-server, across a reload and another browser', async ({ page, browser }) => {
  await twoProjects(page)
  // Start from the backend's default, whatever an earlier run saved.
  const tenants = (await (await page.request.get('/api/v1/me/tenants')).json()) as { items: { id: string }[] }
  expect((await page.request.put(`/api/v2/tenants/${tenants.items[0].id}/chat/project-order`, { data: { projectIds: [] } })).ok()).toBe(true)
  await page.evaluate(() => localStorage.clear())
  await page.reload()
  await expect.poll(() => order(page)).toEqual(['项目 cloud', '项目 desktop'])

  // Drag desktop by its handle above cloud.
  const handle = page.getByRole('button', { name: '拖动以调整 desktop 的顺序' })
  const target = (await projects(page).filter({ hasText: 'cloud' }).boundingBox())!
  const from = (await handle.boundingBox())!
  const saved = page.waitForResponse((r) => r.url().endsWith('/chat/project-order') && r.request().method() === 'PUT')
  await page.mouse.move(from.x + from.width / 2, from.y + from.height / 2)
  await page.mouse.down()
  await page.mouse.move(from.x + from.width / 2, target.y + 4, { steps: 12 })
  await page.mouse.up()
  expect((await saved).ok()).toBe(true)
  await expect.poll(() => order(page)).toEqual(['项目 desktop', '项目 cloud'])

  // The browser's own copy is not what restores it: a reload without it, and another browser.
  await page.evaluate(() => localStorage.clear())
  await page.reload()
  await expect.poll(() => order(page)).toEqual(['项目 desktop', '项目 cloud'])
  const other = await browser.newContext()
  try {
    const elsewhere = await other.newPage()
    await twoProjects(elsewhere)
    await expect.poll(() => order(elsewhere)).toEqual(['项目 desktop', '项目 cloud'])
  } finally {
    await other.close()
  }
})

test('chat: a real Agent answers, a stopped chat turns read-only, history survives a reload', async ({ page }) => {
  test.skip(!realAgents, 'needs V2_NODE_ASSETS_DIR for a real Node and Agent')
  // The first conversation in a project installs the Agent plugin in the project's Node.
  test.setTimeout(300_000)
  const column = page.getByRole('complementary', { name: '聊天记录' })
  const demo = projects(page).filter({ hasText: 'Demo' })
  const composer = page.getByRole('textbox', { name: '消息' })
  const answer = 'V2 fixture complete. Real Agent executed the shell tool.'
  const ended = page.getByRole('status').filter({ hasText: '这个对话已结束' })
  const stamp = Date.now().toString(36)
  const answered = `读一下 README ${stamp}`
  const stopped = `这一轮会被停止 ${stamp}`
  // The thread, not the header or the history column, which also show the title.
  const thread = page.getByRole('region', { name: '会话' }).getByRole('main')

  await page.goto('/chat')
  // The real-chain spec needs the email login this branch introduced, like every other spec.
  await signIn(page)
  await expect(page.getByRole('heading', { name: '今天想构建什么？' })).toBeVisible()

  // A new chat in the demo project: the fixture model has the real Agent run its shell tool.
  await page.getByRole('button', { name: '选择项目' }).click()
  await page.getByRole('option', { name: 'Demo' }).click()
  await composer.fill(answered)
  await composer.press('Enter')
  await expect(page.getByText(answer)).toBeVisible({ timeout: 240_000 })
  await expect(column.getByRole('button', { name: answered, exact: true })).toBeVisible()

  // A second chat, stopped while its Agent is still starting, is read-only from then on.
  await demo.getByRole('button', { name: '在 Demo 新建对话' }).click()
  await page.getByRole('menuitem', { name: '新建对话' }).click()
  await composer.fill(stopped)
  await composer.press('Enter')
  await page.getByRole('button', { name: '停止', exact: true }).click()
  await expect(page.getByText('已停止 · 这个对话已结束，不能继续追问')).toBeVisible({ timeout: 60_000 })
  await expect(ended).toBeVisible()
  await expect(composer).toBeHidden()
  await expect(thread.getByText(stopped, { exact: true })).toHaveCount(1)

  // After a reload both come back from v2-server as they were.
  await page.reload()
  await column.getByRole('button', { name: answered, exact: true }).click()
  await expect(thread.getByText(answered, { exact: true })).toHaveCount(1)
  await expect(page.getByText(answer)).toBeVisible()
  await expect(composer).toBeVisible()
  await column.getByRole('button', { name: stopped, exact: true }).click()
  // The Agent never started, so no history holds this turn; it comes back from its result.
  await expect(thread.getByText(stopped, { exact: true })).toHaveCount(1)
  await expect(page.getByText('已停止 · 这个对话已结束，不能继续追问')).toBeVisible()
  await expect(ended).toBeVisible()
})

/** The names of the projects v2-server lists for the signed-in person's team. */
async function backendProjects(page: Page): Promise<string[]> {
  const tenants = (await (await page.request.get('/api/v1/me/tenants')).json()) as { items: { id: string }[] }
  const list = (await (await page.request.get(`/api/v2/tenants/${tenants.items[0].id}/projects?limit=100`)).json()) as { items: { name: string }[] }
  return list.items.map((p) => p.name)
}

test('chat: creates a project from a public repository and starts the new chat in it', async ({ page }) => {
  // The backend clones the repository before it answers; this one is small and public.
  test.setTimeout(150_000)
  await page.goto('/chat')
  await signIn(page)
  await expect(page.getByRole('heading', { name: '今天想构建什么？' })).toBeVisible()
  const name = `Hello-World ${Date.now()}`

  await page.getByRole('button', { name: '选择项目' }).click()
  await page.getByRole('option', { name: /新建项目/ }).click()
  const dialog = page.getByRole('dialog', { name: '新建项目' })
  await dialog.getByLabel('仓库地址').fill('https://github.com/octocat/Hello-World.git')
  await expect(dialog.getByLabel('项目名')).toHaveValue('Hello-World')

  // A branch the repository lacks is refused, and no project is recorded.
  await dialog.getByLabel('默认分支').fill('no-such-branch')
  await dialog.getByLabel('项目名').fill(name)
  await dialog.getByRole('button', { name: '新建项目' }).click()
  await expect(dialog.getByRole('alert')).toContainText('仓库里没有分支 no-such-branch', { timeout: 120_000 })
  expect(await backendProjects(page)).not.toContain(name)

  await dialog.getByLabel('默认分支').fill('master')
  await dialog.getByRole('button', { name: '新建项目' }).click()
  await expect(dialog).toBeHidden({ timeout: 120_000 })
  await expect(page.getByRole('region', { name: `项目 ${name}` })).toBeVisible()
  await expect(page.getByRole('button', { name: '选择项目' })).toHaveText(name)
  expect(await backendProjects(page)).toContain(name)
})

test('chat: creates a project from the history column and opens a new chat in it', async ({ page }) => {
  test.setTimeout(150_000)
  await page.goto('/chat')
  await signIn(page)
  await expect(page.getByRole('heading', { name: '今天想构建什么？' })).toBeVisible()
  const name = `Hello-World ${Date.now()}`

  await page.getByRole('complementary', { name: '聊天记录' }).getByRole('button', { name: '新建项目' }).click()
  const dialog = page.getByRole('dialog', { name: '新建项目' })
  await dialog.getByLabel('仓库地址').fill('https://github.com/octocat/Hello-World.git')
  await dialog.getByLabel('默认分支').fill('master')
  await dialog.getByLabel('项目名').fill(name)
  await dialog.getByRole('button', { name: '新建项目' }).click()
  await expect(dialog).toBeHidden({ timeout: 120_000 })
  await expect(page.getByRole('region', { name: `项目 ${name}` })).toBeVisible()
  await expect(page.getByRole('button', { name: '选择项目' })).toHaveText(name)
  await expect(page.getByRole('textbox', { name: '消息' })).toBeFocused()
  expect(await backendProjects(page)).toContain(name)
})

test('chat: a sign-in in another tab replaces the previous account without a reload', async ({ browser }) => {
  test.setTimeout(180_000)
  const context = await browser.newContext()
  const first = await context.newPage()
  const second = await context.newPage()
  const ownerTitle = 'Owner 的对话'
  const memberTitle = '成员自己的对话'
  let history: 'owner' | 'member' = 'owner'
  const project = {
    id: '00000000-0000-4000-8000-0000000000e1',
    tenantId: '00000000-0000-4000-8000-0000000000aa',
    ownerUserId: '00000000-0000-4000-8000-0000000000bb',
    name: 'cloud',
    repositoryUrl: 'https://github.com/ora-space/cloud',
    defaultBranch: 'main',
    commitId: '0'.repeat(40),
    createdAt: '2026-11-03T09:00:00Z',
  }
  const conversation = (id: string, title: string) => ({
    id,
    projectId: project.id,
    executorId: 'oc-fixture',
    title,
    createdAt: '2026-11-03T09:00:00Z',
    updatedAt: '2026-11-03T09:00:00Z',
    session: { id: `${id}-session`, startedAt: '2026-11-03T09:00:00Z', endedAt: null, endReason: '' },
  })

  await context.route('**/api/v2/tenants/*/projects*', (route) =>
    route.fulfill({ json: { items: [project], nextCursor: '' } }),
  )
  await context.route('**/api/v2/tenants/*/chat/executors', (route) =>
    route.fulfill({
      json: {
        items: [{
          id: 'oc-fixture', name: 'OpenCode · Fixture', agentName: 'OpenCode',
          agentId: 'official/ora-space.opencode', modelId: 'fixture', modelName: 'Fixture',
        }],
      },
    }),
  )
  await context.route('**/api/v2/tenants/*/chat/project-order', (route) =>
    route.fulfill({ json: { projectIds: [] } }),
  )
  await context.route('**/api/v2/tenants/*/chat/conversations', (route) =>
    route.fulfill({
      json: {
        items: [history === 'owner' ? conversation('owner-chat', ownerTitle) : conversation('member-chat', memberTitle)],
        nextCursor: '',
      },
    }),
  )

  await first.goto('/chat')
  await signIn(first)
  await expect(first.getByRole('button', { name: ownerTitle, exact: true })).toBeVisible()

  await second.goto('/chat')
  await expect(second.getByRole('button', { name: '退出登录', exact: true })).toBeVisible()
  const suffix = crypto.randomUUID().slice(0, 8)
  const email = `member-${suffix}@demo.ora-space.dev`
  const name = `受邀成员-${suffix}`
  const tenants = await (await second.request.get('/api/v1/me/tenants')).json()
  const invited = await second.request.post(`/api/v2/tenants/${tenants.items[0].id}/members`, {
    data: { email, displayName: name },
  })
  expect(invited.ok()).toBeTruthy()

  await second.getByRole('button', { name: '退出登录', exact: true }).click()
  await expect(second.getByRole('heading', { name: '登录 Ora' })).toBeVisible()
  history = 'member'
  await signIn(second, email)

  // The first tab revalidates from the storage signal; no reload is needed.
  await expect(first.getByRole('img', { name })).toBeVisible({ timeout: 10_000 })
  await expect(first.getByRole('button', { name: memberTitle, exact: true })).toBeVisible()
  await expect(first.getByRole('button', { name: ownerTitle, exact: true })).toHaveCount(0)

  await first.reload()
  await expect(first.getByRole('img', { name })).toBeVisible()
  await expect(first.getByRole('button', { name: memberTitle, exact: true })).toBeVisible()
  await expect(first.getByRole('button', { name: ownerTitle, exact: true })).toHaveCount(0)

  await context.close()
})
