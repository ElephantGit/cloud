/// <reference lib="dom" />
import { expect, test } from '@playwright/test'
import { seedWorkspaceMember } from './roster.ts'

test('create a squad, add and remove a member, then archive it', async ({ page }) => {
  const suffix = crypto.randomUUID().slice(0, 8)
  const name = `演示小队-${suffix}`
  const colleague = `同事-${suffix}`
  await page.goto('/multica')
  await page.getByRole('button', { name: '开发者登录' }).click()
  await expect(page.getByRole('heading', { level: 1, name: 'Multica' })).toBeVisible()
  // The demo login creates the workspace; the second human has to exist before the picker opens.
  await seedWorkspaceMember(colleague)

  // The creator becomes the squad's first human maintainer; the counts come from the backend.
  await page.getByRole('button', { name: '新建小队' }).click()
  const dialog = page.getByRole('dialog')
  await dialog.getByLabel('小队名称').fill(name)
  await dialog.getByLabel('简介').fill('由 e2e 创建的演示小队')
  await dialog.getByLabel('协作说明').fill('人类定优先级，Agent 收集数据。')
  await dialog.getByRole('button', { name: '创建小队' }).click()
  await expect(dialog.getByRole('heading', { name })).toBeVisible()
  await expect(dialog.getByText(/人类\s*1/)).toBeVisible()
  await expect(dialog.getByText(/维护者\s*1/)).toBeVisible()
  await expect(dialog.getByText('演示用户')).toBeVisible()

  // An existing workspace member joins the squad with a role; the picker never offers the creator.
  await dialog.getByRole('button', { name: '添加成员' }).click()
  const picker = page.getByRole('dialog')
  await expect(picker.getByText('演示用户')).toHaveCount(0)
  await picker.getByLabel('职责（可选）').fill('外部协作')
  await page.screenshot({ path: test.info().outputPath('picker.png'), fullPage: true })
  await picker.getByRole('listitem').filter({ hasText: colleague }).getByRole('button', { name: '加入' }).click()
  await expect(page.getByText(`已加入 ${colleague}`)).toBeVisible()

  // The space's installed agent joins as an agent member; joining only stores the grouping.
  const agents = picker.getByRole('region', { name: '可加入的空间 Agent' })
  await expect(agents.getByText('official/ora-space.opencode')).toBeVisible()
  await expect(agents.getByText('已安装 0.6.11')).toBeVisible()
  await picker.getByLabel('职责（可选）').fill('实现')
  await page.screenshot({ path: test.info().outputPath('picker-agents.png'), fullPage: true })
  await agents.getByRole('button', { name: '加入' }).click()
  await expect(page.getByText('已加入 ora-space.opencode')).toBeVisible()
  await picker.getByRole('button', { name: '完成' }).click()
  const detail = page.getByRole('dialog')
  await expect(detail.getByText(colleague)).toBeVisible()
  await expect(detail.getByText('外部协作')).toBeVisible()
  await expect(detail.getByText('OpenCode')).toBeVisible()
  await expect(detail.getByText('实现')).toBeVisible()
  await expect(detail.getByText(/人类\s*2/)).toBeVisible()
  await expect(detail.getByText(/Agent\s*1/)).toBeVisible()
  await expect(detail.getByText(/不授予私人模型、Git 凭据或运行时权限/)).toBeVisible()
  await page.screenshot({ path: test.info().outputPath('detail.png'), fullPage: true })
  await page.keyboard.press('Escape')

  // The relation survives a reload, and the list reports the same live counts.
  await page.reload()
  const card = page.getByRole('button', { name: `查看${name}` })
  await expect(card).toBeVisible()
  await expect(card).toContainText('人类 2')
  await expect(card).toContainText('Agent 1')
  await expect(card).toContainText('维护者 1')
  await page.screenshot({ path: test.info().outputPath('list.png'), fullPage: true })
  await card.click()
  await expect(detail.getByRole('heading', { name })).toBeVisible()

  // Removing a member ends only the squad membership and keeps the history.
  await detail.getByRole('button', { name: `移除${colleague}` }).click()
  await detail.getByRole('button', { name: '确认移除' }).click()
  await expect(page.getByText(`已移除 ${colleague}`)).toBeVisible()
  await expect(detail.getByText('已退出 1')).toBeVisible()
  await expect(detail.getByText(/人类\s*1/)).toBeVisible()
  // The agent membership is untouched by the human removal.
  await expect(detail.getByText(/Agent\s*1/)).toBeVisible()
  await expect(detail.getByText('OpenCode')).toBeVisible()
  await page.screenshot({ path: test.info().outputPath('members.png'), fullPage: true })

  // Archiving needs confirmation, ends the live membership and keeps the history read-only.
  await detail.getByRole('button', { name: '归档' }).click()
  await detail.getByRole('button', { name: '确认归档' }).click()
  await expect(detail.getByText('已归档 · 只读')).toBeVisible()
  await expect(detail.getByText(/这个小队已归档/)).toBeVisible()
  await expect(detail.getByRole('button', { name: '编辑' })).toHaveCount(0)
  await page.keyboard.press('Escape')
  await expect(page.getByRole('button', { name: `查看${name}` })).toHaveCount(0)

  // The layout holds at phone width without horizontal scrolling.
  await page.setViewportSize({ width: 390, height: 844 })
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await page.screenshot({ path: test.info().outputPath('mobile.png'), fullPage: true })
})
