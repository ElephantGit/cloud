/// <reference lib="dom" />
import { expect, test } from '@playwright/test'

test('create a squad, read its live members, and archive it', async ({ page }) => {
  const suffix = crypto.randomUUID().slice(0, 8)
  const name = `演示小队-${suffix}`
  await page.goto('/multica')
  await page.getByRole('button', { name: '开发者登录' }).click()
  await expect(page.getByRole('heading', { level: 1, name: 'Multica' })).toBeVisible()

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
  await expect(dialog.getByText(/不授予私人模型、Git 凭据或运行时权限/)).toBeVisible()
  await page.screenshot({ path: test.info().outputPath('detail.png'), fullPage: true })
  await page.keyboard.press('Escape')

  // The relation survives a reload, and the list reports the same live counts.
  await page.reload()
  const card = page.getByRole('button', { name: `查看${name}` })
  await expect(card).toBeVisible()
  await expect(card).toContainText('人类 1')
  await expect(card).toContainText('维护者 1')
  await page.screenshot({ path: test.info().outputPath('list.png'), fullPage: true })
  await card.click()
  await expect(dialog.getByRole('heading', { name })).toBeVisible()

  // Archiving needs confirmation, ends the live membership and keeps the history read-only.
  await dialog.getByRole('button', { name: '归档' }).click()
  await dialog.getByRole('button', { name: '确认归档' }).click()
  await expect(dialog.getByText('已归档 · 只读')).toBeVisible()
  await expect(dialog.getByText(/这个小队已归档/)).toBeVisible()
  await expect(dialog.getByRole('button', { name: '编辑' })).toHaveCount(0)
  await page.keyboard.press('Escape')
  await expect(page.getByRole('button', { name: `查看${name}` })).toHaveCount(0)

  // The layout holds at phone width without horizontal scrolling.
  await page.setViewportSize({ width: 390, height: 844 })
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await page.screenshot({ path: test.info().outputPath('mobile.png'), fullPage: true })
})
