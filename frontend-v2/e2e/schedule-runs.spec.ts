/// <reference lib="dom" />
import { expect, test, type Page } from '@playwright/test'
import { signIn } from './login.ts'

/**
 * These specs run real executions instead of simulated receipts: the fixture model answer only
 * appears when the shared model turn (or the real Agent session) actually ran, and the history is
 * read back from PostgreSQL through the operation and history APIs.
 */
const modelAnswer = 'V2 fixture response.'
const agentAnswer = 'V2 fixture complete. Real Agent executed the shell tool.'
/** Agent runs need the prepared Node assets; CI without them keeps the Agent list empty. */
const nodeAssets = process.env.V2_NODE_ASSETS_DIR
const agentTimeout = 240_000

async function openSchedules(page: Page) {
  await page.goto('/schedules')
  await signIn(page)
  await expect(page.getByRole('heading', { name: '定时任务', exact: true })).toBeVisible()
}

async function createSchedule(page: Page, name: string, prompt: string, agent = false) {
  const dialog = page.getByRole('dialog')
  await page.locator('header').getByRole('button', { name: '新建任务', exact: true }).click()
  await dialog.getByRole('textbox', { name: '任务名称' }).fill(name)
  await dialog.getByRole('textbox', { name: '任务指令' }).fill(prompt)
  if (agent) {
    await dialog.getByRole('combobox', { name: '关联项目', exact: true }).selectOption({ label: 'Demo' })
    await dialog.getByRole('combobox', { name: 'Agent', exact: true }).selectOption({ label: 'OpenCode' })
    await dialog.getByRole('combobox', { name: '模型', exact: true }).selectOption({ label: 'Fixture' })
  } else {
    await dialog.getByRole('combobox', { name: '模型', exact: true }).selectOption('fixture')
  }
  await dialog.getByRole('button', { name: '创建并启用' }).click()
  await expect(dialog.getByRole('heading', { name })).toBeVisible()
  return dialog
}

/** One manual run, observed in the history tab the page switches to after the request. */
async function runNow(dialog: ReturnType<Page['getByRole']>) {
  await dialog.getByRole('button', { name: '立即运行', exact: true }).click()
  await expect(dialog.getByRole('button', { name: '运行历史' })).toHaveAttribute('aria-pressed', 'true')
  await expect(dialog.getByText('运行请求已记录，当前状态见运行历史。')).toBeVisible()
}

test('立即运行 executes the shared model turn and its result stays in history after reload', async ({ page }) => {
  const name = `真实模型运行-${crypto.randomUUID().slice(0, 8)}`
  await openSchedules(page)
  const dialog = await createSchedule(page, name, '用三句话总结本次验收')
  await runNow(dialog)
  // The history row is the receipt the server recorded; the answer proves the turn completed.
  await expect(dialog.getByText('成功', { exact: true }).first()).toBeVisible({ timeout: 20_000 })
  await expect(dialog.getByText('完整结果')).toBeVisible({ timeout: 20_000 })
  await expect(dialog.getByText('手动').first()).toBeVisible()
  await expect(dialog.getByText(modelAnswer, { exact: true })).toBeVisible()
  await page.screenshot({ path: test.info().outputPath('schedule-model-run.png'), fullPage: true })

  // A reload reads the same durable records back; no run is replayed by refreshing.
  await page.reload()
  await expect(page.getByRole('heading', { name: '定时任务', exact: true })).toBeVisible()
  await page.getByRole('button', { name: `查看${name}` }).click()
  await dialog.getByRole('button', { name: '运行历史' }).click()
  await expect(dialog.getByText('成功', { exact: true }).first()).toBeVisible({ timeout: 20_000 })
  await expect(dialog.getByText('完整结果')).toBeVisible({ timeout: 20_000 })
  await expect(dialog.getByText(modelAnswer, { exact: true })).toBeVisible()
  await page.screenshot({ path: test.info().outputPath('schedule-model-history-after-reload.png'), fullPage: true })
})

test('a project Agent run keeps its real tool-backed answer in history', async ({ page }) => {
  test.skip(!nodeAssets, 'Requires prepared Node assets (task v2:node:prepare) and V2_NODE_ASSETS_DIR')
  test.setTimeout(agentTimeout)
  const name = `真实Agent运行-${crypto.randomUUID().slice(0, 8)}`
  await openSchedules(page)
  const dialog = await createSchedule(page, name, '列出当前目录的文件，并说明你用的模型', true)
  await runNow(dialog)
  await expect(dialog.getByText('成功', { exact: true }).first()).toBeVisible({ timeout: agentTimeout - 20_000 })
  await expect(dialog.getByText('完整结果')).toBeVisible({ timeout: 30_000 })
  await expect(dialog.getByText(agentAnswer, { exact: true })).toBeVisible()
  await page.screenshot({ path: test.info().outputPath('schedule-agent-run.png'), fullPage: true })
})

test('cancelling a running Agent schedule keeps a cancelled record and does not fake success', async ({ page }) => {
  test.skip(!nodeAssets, 'Requires prepared Node assets (task v2:node:prepare) and V2_NODE_ASSETS_DIR')
  test.setTimeout(agentTimeout)
  const name = `取消运行-${crypto.randomUUID().slice(0, 8)}`
  await openSchedules(page)
  const dialog = await createSchedule(page, name, '逐个检查项目文件', true)
  await runNow(dialog)
  // The cancel action stays available only while the run is still unreleased. Stopping an Agent
  // session takes a while, and a periodic scan may hold the occurrence; the documented retry is the
  // same action with the same occurrence ID, never a newly created run.
  const cancel = dialog.getByRole('button', { name: '取消此次运行' })
  const stopped = dialog.getByText('取消请求已处理，当前状态见运行历史。')
  const held = dialog.getByText('此运行正在处理，请稍后重试同一操作。')
  await expect(cancel).toBeVisible({ timeout: 60_000 })
  await cancel.click()
  await expect(stopped.or(held)).toBeVisible({ timeout: 90_000 })
  if (await held.isVisible()) await cancel.click()
  await expect(stopped).toBeVisible({ timeout: 90_000 })
  await expect(dialog.getByText('已取消', { exact: true }).first()).toBeVisible({ timeout: 120_000 })
  await expect(dialog.getByText('完整结果')).toHaveCount(0)
  await page.screenshot({ path: test.info().outputPath('schedule-agent-cancelled.png'), fullPage: true })
})
