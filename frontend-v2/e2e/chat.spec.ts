import { expect, type Page, test } from '@playwright/test'

const projects = (page: Page) => page.getByRole('complementary', { name: '聊天记录' }).getByRole('region')

test('chat: replay a sample, talk in a new session, reorder projects', async ({ page }) => {
  await page.goto('/chat')
  await page.getByRole('button', { name: '开发者登录' }).click()
  await expect(page.getByRole('heading', { name: '今天想构建什么？' })).toBeVisible()

  // A sample replays its recording: tool calls stream in, then fold into a summary.
  await projects(page).filter({ hasText: '示例' }).getByRole('button', { name: '工具调用：读、搜、改、执行', exact: true }).click()
  await expect(page.getByText('运行 npm run check')).toBeVisible()
  await expect(page.getByText('部分操作失败')).toBeVisible({ timeout: 20_000 })
  await expect(page.getByText('修改了 1 个文件')).toBeVisible()

  // A new chat in a project's worktree becomes a session with its first prompt, answered by the
  // stand-in reply until v2-server runs agents.
  const cloud = projects(page).filter({ hasText: 'ora-space/cloud' })
  await cloud.getByRole('button', { name: '在 ora-space/cloud 新建对话' }).click()
  await page.getByRole('menuitem', { name: '新建 worktree 并命名…' }).click()
  await page.getByRole('textbox', { name: '分支名' }).fill('feat/e2e')
  await page.getByRole('button', { name: '创建' }).click()
  await expect(page.getByRole('button', { name: '选择工作目录' })).toHaveText('feat/e2e')
  await page.getByRole('textbox', { name: '消息' }).fill('你好')
  await page.keyboard.press('Enter')
  await expect(page.getByText(/这是演示回复/)).toBeVisible()
  await expect(page.getByTitle('Agent 在独立的 worktree 里工作')).toHaveText('feat/e2e')
  await expect(cloud.getByLabel('worktree feat/e2e').getByText('你好', { exact: true })).toBeVisible()

  // Dragging a project by its handle moves it, and the order survives a reload.
  const names = () => projects(page).evaluateAll((els) => els.map((e) => e.getAttribute('aria-label')))
  expect(await names()).toEqual(['项目 示例', '项目 ora-space/cloud', '项目 ora-space/desktop'])
  const desktop = projects(page).filter({ hasText: 'ora-space/desktop' })
  await desktop.hover()
  const handle = desktop.getByRole('button', { name: '拖动以调整 ora-space/desktop 的顺序' })
  const from = (await handle.boundingBox())!
  const to = (await projects(page).first().boundingBox())!
  await page.mouse.move(from.x + from.width / 2, from.y + from.height / 2)
  await page.mouse.down()
  await page.mouse.move(from.x + from.width / 2, to.y + 4, { steps: 12 })
  // Only the row follows the pointer; the project itself stays put, greyed, until the drop.
  await expect(desktop).toHaveAttribute('data-dragging', 'true')
  await page.mouse.up()
  await expect.poll(names).toEqual(['项目 ora-space/desktop', '项目 示例', '项目 ora-space/cloud'])
  await page.reload()
  await expect.poll(names).toEqual(['项目 ora-space/desktop', '项目 示例', '项目 ora-space/cloud'])

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
