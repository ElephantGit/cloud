import { expect, test } from '@playwright/test'

test('log in, switch pages, log out', async ({ page }) => {
  // Signed out: the login screen appears in place of the requested page.
  await page.goto('/workflows')
  await expect(page.getByRole('heading', { name: '登录 Ora' })).toBeVisible()

  // The development login returns to the requested page as the demo user.
  await page.getByRole('button', { name: '开发者登录' }).click()
  await expect(page).toHaveURL(/\/workflows$/)
  await expect(page.getByRole('heading', { level: 1, name: '工作流' })).toBeVisible()
  await expect(page.getByRole('img', { name: '演示用户' })).toBeVisible()

  for (const title of ['对话', '协作', '定时任务', '仪表盘']) {
    await page.getByRole('link', { name: title }).click()
    await expect(page.getByRole('heading', { level: 1, name: title })).toBeVisible()
  }

  // The session survives a reload, and logging out ends it.
  await page.reload()
  await expect(page.getByRole('heading', { level: 1, name: '仪表盘' })).toBeVisible()
  await page.getByRole('button', { name: '退出登录' }).click()
  await expect(page.getByRole('heading', { name: '登录 Ora' })).toBeVisible()
  await page.reload()
  await expect(page.getByRole('heading', { name: '登录 Ora' })).toBeVisible()
})
