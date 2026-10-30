import { expect, type Page, test } from '@playwright/test'

const projects = (page: Page) => page.getByRole('complementary', { name: '聊天记录' }).getByRole('region')

test('chat: replay a sample, say the backend cannot run Agents yet, resize the column', async ({ page }) => {
  await page.goto('/chat')
  await page.getByRole('button', { name: '开发者登录' }).click()
  await expect(page.getByRole('heading', { name: '今天想构建什么？' })).toBeVisible()

  // v2-server has no execution runtime wired yet, so its chat routes answer 503 and the new chat
  // says so instead of offering projects it cannot run.
  await expect(page.getByRole('status').filter({ hasText: /后端还不能运行 Agent/ })).toBeVisible()
  expect(await projects(page).evaluateAll((els) => els.map((e) => e.getAttribute('aria-label')))).toEqual(['项目 示例'])

  // A sample replays its recording: tool calls stream in, then fold into a summary.
  await projects(page).filter({ hasText: '示例' }).getByRole('button', { name: '工具调用：读、搜、改、执行', exact: true }).click()
  await expect(page.getByText('运行 npm run check')).toBeVisible()
  await expect(page.getByText('部分操作失败')).toBeVisible({ timeout: 20_000 })
  await expect(page.getByText('修改了 1 个文件')).toBeVisible()

  // Typing into a sample gets the stand-in reply, which says it is one.
  await page.getByRole('textbox', { name: '消息' }).fill('你好')
  await page.keyboard.press('Enter')
  await expect(page.getByText(/这是演示回复/)).toBeVisible()

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
