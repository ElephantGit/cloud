import { expect, test } from '@playwright/test'
import { ownerName, signIn } from './login.ts'

test('log in, switch pages, log out', async ({ page }) => {
  // Signed out: the login screen appears in place of the requested page.
  await page.goto('/workflows')
  await expect(page.getByRole('heading', { name: '登录 Ora' })).toBeVisible()

  // The email login returns to the requested page as the preset owner.
  await signIn(page)
  await expect(page).toHaveURL(/\/workflows$/)
  await expect(page.getByRole('heading', { level: 1, name: '工作流' })).toBeVisible()
  await expect(page.getByRole('img', { name: ownerName })).toBeVisible()

  for (const title of ['对话', '协作', '定时任务', 'Dashboard']) {
    await page.getByRole('link', { name: title }).click()
    await expect(page.getByRole('heading', { level: 1, name: title })).toBeVisible()
  }

  // The session survives a reload, and logging out ends it.
  await page.reload()
  await expect(page.getByRole('heading', { level: 1, name: 'Dashboard' })).toBeVisible()
  await page.getByRole('button', { name: '退出登录' }).click()
  await expect(page.getByRole('heading', { name: '登录 Ora' })).toBeVisible()
  await page.reload()
  await expect(page.getByRole('heading', { name: '登录 Ora' })).toBeVisible()
})
