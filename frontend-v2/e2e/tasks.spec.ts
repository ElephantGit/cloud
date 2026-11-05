/// <reference lib="dom" />
import { expect, test } from '@playwright/test'
import { ownerName, signIn, signInAsInvitedMember } from './login.ts'

// With prepared Node assets (`task v2:node:prepare`, then V2_NODE_ASSETS_DIR) v2-server runs real
// Agents on the fixture model; without them, as in CI, the Agent spec skips itself.
const realAgents = !!process.env.V2_NODE_ASSETS_DIR

test('create a task, assign it, and walk it through the lifecycle', async ({ page }) => {
  const title = `e2e 任务-${crypto.randomUUID().slice(0, 8)}`
  await page.goto('/collaboration')
  await signIn(page)
  await expect(page.getByRole('heading', { level: 1, name: '协作' })).toBeVisible()
  await expect(page.getByRole('navigation', { name: '任务视图' }).getByRole('link', { name: /全部任务/ })).toBeVisible()

  // A task always belongs to a project; the form waits for the demo project rather than inventing one.
  await page.getByRole('main').getByRole('button', { name: '新建任务' }).click()
  const dialog = page.getByRole('dialog')
  await dialog.getByLabel('任务标题').fill(title)
  await dialog.getByLabel('描述（可选）').fill('由 e2e 创建的任务')
  await expect(dialog.getByLabel('所属项目')).not.toHaveValue('')
  await page.screenshot({ path: test.info().outputPath('task-create.png'), fullPage: true })
  await dialog.getByRole('button', { name: '创建任务' }).click()

  // Creating opens the task's own page, a link teammates can share.
  await expect(page).toHaveURL(/\/collaboration\/tasks\//)
  await expect(page.getByRole('heading', { level: 2, name: title })).toBeVisible()
  const side = page.getByRole('complementary', { name: '任务属性' })
  await side.getByLabel('指派给').selectOption({ label: ownerName })
  await expect(side.getByText(new RegExp(`当前：${ownerName}（人）`))).toBeVisible()

  // A person's task moves by hand, one step at a time, and ends terminal.
  await side.getByRole('button', { name: '开始处理' }).click()
  await side.getByRole('button', { name: '提交验收' }).click()
  await side.getByRole('button', { name: '确认完成' }).click()
  await expect(side.getByText('任务已结束，不能再改状态。')).toBeVisible()

  // The team discusses on the task; the comment stays after a reload.
  await page.getByLabel('评论或给 Agent 的消息').fill('e2e 评论：可以收尾了')
  await page.getByRole('button', { name: '评论', exact: true }).click()
  await expect(page.getByRole('list', { name: '讨论' })).toContainText('e2e 评论：可以收尾了')
  await page.screenshot({ path: test.info().outputPath('task-detail.png'), fullPage: true })
  await page.reload()
  await page.getByRole('tab', { name: /讨论/ }).click()
  await expect(page.getByRole('list', { name: '讨论' })).toContainText('e2e 评论：可以收尾了')

  // Back on the tasks the board, the default, shows it in its column; the list shows its final status.
  await page.getByRole('link', { name: '返回任务列表' }).click()
  const done = page.getByRole('listitem', { name: '已完成列' })
  await expect(done).toContainText(title)
  await expect(done.getByRole('button', { name: `查看任务 ${title}` })).toContainText(ownerName)
  await page.screenshot({ path: test.info().outputPath('task-board.png'), fullPage: true })
  await page.getByRole('button', { name: '列表' }).click()
  const row = page.getByRole('button', { name: `查看任务 ${title}` })
  await expect(row).toContainText('已完成')
  await expect(row).toContainText(ownerName)
})

test('drag a card on the board to move it one legal step', async ({ page }) => {
  const title = `e2e 拖动-${crypto.randomUUID().slice(0, 8)}`
  await page.goto('/collaboration?new=1')
  await signIn(page)
  const dialog = page.getByRole('dialog')
  await dialog.getByLabel('任务标题').fill(title)
  await expect(dialog.getByLabel('所属项目')).not.toHaveValue('')
  await dialog.getByRole('button', { name: '创建任务' }).click()
  await expect(page.getByRole('heading', { level: 2, name: title })).toBeVisible()
  await page.getByRole('link', { name: '返回任务列表' }).click()

  // A drag is real pointer movement, so the board's pointer sensor picks the card up as for a person.
  const card = page.getByRole('button', { name: `查看任务 ${title}` })
  const drag = async (column: string) => {
    const from = (await card.boundingBox())!, to = (await page.getByRole('listitem', { name: column }).boundingBox())!
    await page.mouse.move(from.x + from.width / 2, from.y + from.height / 2)
    await page.mouse.down()
    await page.mouse.move(from.x + from.width / 2 + 12, from.y + from.height / 2, { steps: 4 })
    await page.mouse.move(to.x + to.width / 2, to.y + 80, { steps: 12 })
    await page.mouse.up()
  }
  await drag('进行中列')
  await expect(page.getByRole('listitem', { name: '进行中列' })).toContainText(title)
  // Skipping 待验收 is refused with the reason, and the card stays where it was, also after a reload.
  await drag('已完成列')
  await expect(page.getByText('逐步推进')).toBeVisible()
  await page.screenshot({ path: test.info().outputPath('board-drag.png'), fullPage: true })
  await page.reload()
  await expect(page.getByRole('listitem', { name: '进行中列' })).toContainText(title)
})

test('an Agent runs a task, the team watches, only the starter follows up', async ({ page, browser }) => {
  test.skip(!realAgents, 'needs V2_NODE_ASSETS_DIR for a real Node and Agent')
  // The first run in a project installs the Agent plugin in the project's Node.
  test.setTimeout(360_000)
  const title = `e2e Agent 任务-${crypto.randomUUID().slice(0, 8)}`
  const answer = 'V2 fixture complete. Real Agent executed the shell tool.'
  await page.goto('/collaboration')
  await signIn(page)

  await page.getByRole('main').getByRole('button', { name: '新建任务' }).click()
  const dialog = page.getByRole('dialog')
  await dialog.getByLabel('任务标题').fill(title)
  await dialog.getByLabel('描述（可选）').fill('读一下 README，告诉我项目做什么')
  await expect(dialog.getByLabel('所属项目')).not.toHaveValue('')
  await dialog.getByRole('button', { name: '创建任务' }).click()
  const side = page.getByRole('complementary', { name: '任务属性' })
  await side.getByLabel('指派给').selectOption({ label: 'OpenCode' })

  // The owner starts it on the fixture model; the run is theirs, the task enters 进行中.
  await side.getByLabel('执行模型').selectOption({ label: 'OpenCode / Fixture' })
  await side.getByRole('button', { name: '开始处理' }).click()
  await expect(page.getByRole('region', { name: '运行与讨论' }).getByText(answer)).toBeVisible({ timeout: 300_000 })
  // A finished turn hands the task to review by itself.
  await expect(page.locator('.mc-task-bar .mc-status')).toHaveText('待验收', { timeout: 30_000 })
  await page.screenshot({ path: test.info().outputPath('task-run.png'), fullPage: true })
  const url = page.url()

  // A colleague sees the same task and run, may comment, but cannot spend the owner's identity.
  const colleague = await (await browser.newContext()).newPage()
  await signInAsInvitedMember(colleague, url)
  await expect(colleague.getByRole('region', { name: '运行与讨论' }).getByText(answer)).toBeVisible({ timeout: 60_000 })
  await expect(colleague.getByRole('switch', { name: '发给 Agent' })).toHaveAttribute('aria-disabled', 'true')
  await expect(colleague.getByText(/只有发起人能追问/).first()).toBeVisible()
  await colleague.getByLabel('评论或给 Agent 的消息').fill('看起来可以，等 Owner 确认')
  await colleague.getByRole('button', { name: '评论', exact: true }).click()
  await expect(colleague.getByRole('list', { name: '讨论' })).toContainText('看起来可以，等 Owner 确认')
  await colleague.screenshot({ path: test.info().outputPath('task-run-colleague.png'), fullPage: true })

  // The owner follows up in the same run, which is where a message goes by default, then stops it;
  // the task returns to 待处理.
  await expect(page.getByRole('switch', { name: '发给 Agent' })).toHaveAttribute('aria-checked', 'true')
  await page.getByLabel('评论或给 Agent 的消息').fill('再确认一次')
  await page.getByRole('button', { name: '发给 Agent', exact: true }).click()
  await expect(page.locator('.mc-task-bar .mc-status')).toHaveText(/进行中|待验收/)
  await page.getByRole('button', { name: /停止|结束会话/ }).click()
  await expect(page.getByLabel('运行概况').getByText('已停止')).toBeVisible({ timeout: 60_000 })
  await page.screenshot({ path: test.info().outputPath('task-run-stopped.png'), fullPage: true })
})
