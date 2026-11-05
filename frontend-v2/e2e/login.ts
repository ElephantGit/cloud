import { expect, type Page } from '@playwright/test'

/** The owner from `v2-server/users.json`, the account the startup preset guarantees. */
export const ownerEmail = 'owner@demo.ora-space.dev'
/** The owner's display name in `v2-server/users.json`; the roster shows it instead of an address. */
export const ownerName = '协作空间 Owner'

/**
 * Signs in through the real login screen. Every test gets a private browser context, so this is
 * the only way a spec becomes authenticated; it waits for the shell that only a session unlocks,
 * not for a particular account's name.
 */
export async function signIn(page: Page, email = ownerEmail) {
  await page.getByLabel('邮箱').fill(email)
  await page.getByRole('button', { name: '登录', exact: true }).click()
  await expect(
    page.getByRole('button', { name: '退出登录', exact: true }),
  ).toBeVisible()
}

/** Creates a real invited member and signs the page in as that account. */
export async function signInAsInvitedMember(
  page: Page,
  path = '/settings/squads',
) {
  const suffix = crypto.randomUUID().slice(0, 8)
  const email = `member-${suffix}@demo.ora-space.dev`
  const name = `受邀成员-${suffix}`
  // The members page has a 邮箱 field of its own (the invitation form), so the session is resolved
  // on the root page, where the only 邮箱 belongs to the login form.
  await page.goto('/')
  const signOut = page.getByRole('button', { name: '退出登录', exact: true })
  await expect(
    signOut.or(page.getByRole('button', { name: '登录', exact: true })),
  ).toBeVisible()
  if (await signOut.isVisible()) await signOut.click()
  await signIn(page)
  const tenants = await (await page.request.get('/api/v1/me/tenants')).json()
  const invited = await page.request.post(
    `/api/v2/tenants/${tenants.items[0].id}/members`,
    { data: { email, displayName: name } },
  )
  expect(invited.ok()).toBeTruthy()
  await signOut.click()
  await page.goto(path)
  await signIn(page, email)
  return { email, name }
}
