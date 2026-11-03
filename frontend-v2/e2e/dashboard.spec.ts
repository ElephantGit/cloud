import { expect, test } from '@playwright/test'
import { signIn } from './login.ts'

test('dashboard analysis reads the real server with mock disabled', async ({ page }) => {
  await page.goto('/dashboard')
  await signIn(page)
  await page.getByRole('navigation', { name: '仪表盘视图' }).getByRole('link', { name: '会话分析' })
    .click()
  await expect(page.getByRole('heading', { level: 1, name: '会话分析' })).toBeVisible()
  await expect(page.getByText('正在读取会话…')).not.toBeVisible()
  await expect(page.getByRole('list', { name: 'Agent 会话' })).toBeVisible()
  await page.getByLabel('Agent 标识').fill(`absent-${crypto.randomUUID()}`)
  await page.getByRole('button', { name: '应用筛选' }).click()
  await expect(page.getByText('暂无符合条件的任务 Agent 会话。')).toBeVisible()
  await page.reload()
  await expect(page.getByLabel('Agent 标识')).not.toHaveValue('')
})

test('real Agent task history and model requests appear in dashboard', async ({ page }) => {
  test.skip(!process.env.V2_NODE_ASSETS_DIR, 'requires real Node/Agent assets')
  test.setTimeout(360_000)
  const title = `dashboard Agent-${crypto.randomUUID().slice(0, 8)}`
  await page.goto('/collaboration')
  await signIn(page)
  await page.getByRole('main').getByRole('button', { name: '新建任务' }).click()
  const dialog = page.getByRole('dialog')
  await dialog.getByLabel('任务标题').fill(title)
  await dialog.getByLabel('描述（可选）').fill('读取 README 并检查项目。')
  await expect(dialog.getByLabel('所属项目')).not.toHaveValue('')
  await dialog.getByRole('button', { name: '创建任务' }).click()
  const side = page.getByRole('complementary', { name: '任务属性' })
  await side.getByLabel('指派给').selectOption({ label: 'OpenCode' })
  await side.getByLabel('执行模型').selectOption({ label: 'OpenCode / Fixture' })
  await side.getByRole('button', { name: '开始处理' }).click()
  await expect(
    page.getByRole('region', { name: '运行与讨论' }).getByText(
      'V2 fixture complete. Real Agent executed the shell tool.',
    ),
  ).toBeVisible({ timeout: 300_000 })
  const taskUrl = page.url()
  await page.goto('/dashboard/sessions')
  await page.getByRole('link', { name: title, exact: true }).click()
  await expect(page.getByRole('region', { name: '会话概览' })).toBeVisible()
  await page.getByRole('button', { name: '时间轴', exact: true }).click()
  await expect(page.getByRole('region', { name: '会话时间轴' })).toBeVisible()
  await page.screenshot({ path: test.info().outputPath('dashboard-timeline.png'), fullPage: true })
  await page.getByRole('button', { name: '模型请求', exact: true }).click()
  await expect(page.getByRole('region', { name: '模型请求明细' })).toBeVisible()
  await expect(page.getByRole('region', { name: '模型请求明细' })).toContainText('fixture 测试模型')
  await page.screenshot({ path: test.info().outputPath('dashboard-requests.png'), fullPage: true })
  await page.reload()
  await expect(page.getByRole('heading', { name: title })).toBeVisible()
  await page.getByRole('link', { name: '返回来源' }).click()
  await expect(page).toHaveURL(taskUrl)
})
