import { expect, test, type Page } from '@playwright/test'

async function login(page: Page, path: string) {
  await page.goto(path)
  await page.getByRole('button', { name: '开发者登录' }).click()
  // The login screen keeps the requested URL. Its equality alone does not mean the
  // authentication redirect has finished; wait for the newly authenticated shell.
  await expect(page.getByRole('img', { name: '演示用户', exact: true })).toBeVisible()
  await expect(page).toHaveURL(new RegExp(path.replace(/[?]/g, '\\?') + '$'))
}
async function theme(page: Page, name: '浅色' | '深色' | '跟随系统') {
  await page.goto('/settings/theme')
  await page.getByRole('button', { name, exact: true }).click()
}
const titles = [
  ['members', '成员'],
  ['model-connections', '模型连接'],
  ['theme', '主题'],
  ['plugins', 'Plugins'],
  ['skills', 'Skills'],
] as const

test('settings subroutes, refresh, history and the existing shell remain navigable', async ({
  page,
}) => {
  await login(page, '/settings/plugins')
  await page.getByRole('link', { name: '设置', exact: true }).click()
  await expect(page).toHaveURL(/\/settings\/members$/)
  for (const [path, title] of titles) {
    await page
      .getByRole('navigation', { name: '设置导航' })
      .getByRole('link', { name: title, exact: true })
      .click()
    await expect(page).toHaveURL(new RegExp('/settings/' + path + '$'))
    await expect(
      page.getByRole('heading', { level: 1, name: title, exact: true }),
    ).toBeVisible()
    await page.reload()
    await expect(
      page.getByRole('heading', { level: 1, name: title, exact: true }),
    ).toBeVisible()
  }
  await page.goBack()
  await expect(
    page.getByRole('heading', { level: 1, name: 'Plugins', exact: true }),
  ).toBeVisible()
  await page.goForward()
  await expect(
    page.getByRole('heading', { level: 1, name: 'Skills', exact: true }),
  ).toBeVisible()
  for (const title of ['对话', '工作流', '定时任务', '仪表盘']) {
    await page
      .getByRole('navigation', { name: '主导航' })
      .getByRole('link', { name: title, exact: true })
      .click()
    await expect(
      page.getByRole('heading', { level: 1, name: title, exact: true }),
    ).toBeVisible()
  }
})

test('plugin removal and reinstallation update readonly skills and survive reload', async ({
  page,
}) => {
  await login(page, '/settings/plugins')
  const row = page.getByRole('article', { name: '写作工具箱' })
  await row.getByRole('button', { name: '模拟移除' }).click()
  await page
    .getByRole('alertdialog')
    .getByRole('button', { name: '模拟移除' })
    .click()
  await expect(row.getByText('未安装', { exact: true })).toBeVisible()
  await page.goto('/settings/skills')
  const skill = page.getByRole('article', { name: 'writing-guide' })
  await expect(skill.getByText('来源插件未安装')).toBeVisible()
  await page.reload()
  await expect(skill.getByText('来源插件未安装')).toBeVisible()
  await skill.getByRole('button', { name: '查看技能 writing-guide' }).click()
  await expect(
    page.getByRole('dialog').getByRole('button', { name: '编辑技能' }),
  ).toHaveCount(0)
  await page.getByRole('link', { name: /demo\/writing-kit/ }).click()
  await page
    .getByRole('dialog')
    .getByRole('button', { name: '模拟安装' })
    .click()
  await expect(
    page.getByRole('dialog').getByText('已安装', { exact: true }),
  ).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(page.getByRole('dialog')).toHaveCount(0)
  await page.goto('/settings/skills')
  await expect(skill.getByText('可用', { exact: true })).toBeVisible()
})

test('corrupt local data shows a recoverable error and requires reset confirmation', async ({
  page,
}) => {
  await login(page, '/settings/skills')
  await expect(
    page.getByRole('article', { name: 'change-summary' }),
  ).toBeVisible()
  await page.evaluate(() => {
    const key = Object.keys(localStorage).find((k) =>
      k.startsWith('ora-v2-settings-demo:'),
    )!
    localStorage.setItem(key, '{broken')
  })
  await page.reload()
  await expect(page.getByRole('alert')).toContainText('演示数据无法读取')
  await page.getByRole('button', { name: '重试', exact: true }).click()
  await expect(page.getByRole('alert')).toContainText('演示数据无法读取')
  await page.getByRole('button', { name: '重置演示', exact: true }).click()
  await page
    .getByRole('alertdialog')
    .getByRole('button', { name: '取消' })
    .click()
  await expect(page.getByRole('article')).toHaveCount(0)
  await page.getByRole('button', { name: '重置演示', exact: true }).click()
  await page
    .getByRole('alertdialog')
    .getByRole('button', { name: '重置演示', exact: true })
    .click()
  await expect(
    page.getByRole('article', { name: 'change-summary' }),
  ).toBeVisible()
})

test('theme survives reload and follows system changes across existing pages', async ({
  page,
}) => {
  await page.emulateMedia({ colorScheme: 'light' })
  await login(page, '/settings/theme')
  await page.getByRole('button', { name: '深色', exact: true }).click()
  await expect(page.locator('html')).toHaveClass(/dark/)
  await page.reload()
  await expect(page.locator('html')).toHaveClass(/dark/)
  await page.goto('/schedules')
  await expect(
    page.getByRole('heading', { name: '定时任务', exact: true }),
  ).toBeVisible()
  const darkSurface = await page
    .locator('.schedules-feature')
    .evaluate((el) => getComputedStyle(el).backgroundColor)
  await theme(page, '浅色')
  await page.goto('/schedules')
  const lightSurface = await page
    .locator('.schedules-feature')
    .evaluate((el) => getComputedStyle(el).backgroundColor)
  expect(darkSurface).not.toBe(lightSurface)
  await theme(page, '跟随系统')
  await page.emulateMedia({ colorScheme: 'dark' })
  await expect(page.locator('html')).toHaveClass(/dark/)
  await page.emulateMedia({ colorScheme: 'light' })
  await expect(page.locator('html')).not.toHaveClass(/dark/)
})

test('five desktop pages and keyboard-accessible details', async ({
  page,
}, info) => {
  const errors: string[] = []
  page.on('pageerror', (error) => errors.push(error.message))
  await page.setViewportSize({ width: 1440, height: 1000 })
  await login(page, '/settings/plugins')
  await theme(page, '浅色')
  for (const [path, title] of titles) {
    await page.goto('/settings/' + path)
    await expect(
      page.getByRole('heading', { level: 1, name: title, exact: true }),
    ).toBeVisible()
    if (path !== 'theme')
      await expect(page.getByText('正在加载…', { exact: true })).toHaveCount(0)
    await page.screenshot({
      path: info.outputPath(path + '-light.png'),
      fullPage: true,
    })
  }
  await page.goto('/settings/plugins')
  const opener = page.getByRole('button', { name: '查看插件 写作工具箱' })
  await opener.focus()
  await page.keyboard.press('Enter')
  await expect(page.getByRole('dialog')).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(opener).toBeFocused()
  expect(errors).toEqual([])
})

test('dark plugin and skill screenshots', async ({ page }, info) => {
  await page.setViewportSize({ width: 1440, height: 1000 })
  await login(page, '/settings/theme')
  await theme(page, '深色')
  for (const path of ['plugins', 'skills']) {
    await page.goto('/settings/' + path)
    await expect(page.getByRole('article').first()).toBeVisible()
    await page.screenshot({
      path: info.outputPath(path + '-dark.png'),
      fullPage: true,
    })
  }
})

for (const width of [768, 375]) {
  test(`five settings pages at ${width}px keep the active link visible`, async ({
    page,
  }, info) => {
    await page.setViewportSize({ width: 1440, height: 900 })
    await page.emulateMedia({ colorScheme: 'light' })
    await login(page, '/settings/skills')
    await page.setViewportSize({ width, height: 900 })
    await expect(
      page
        .getByRole('navigation', { name: '设置导航' })
        .getByRole('link', { name: 'Skills', exact: true }),
    ).toBeInViewport({ ratio: 1 })
    for (const [path, title] of titles) {
      await page.goto('/settings/' + path)
      await expect(
        page.getByRole('heading', { level: 1, name: title, exact: true }),
      ).toBeVisible()
      expect(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= window.innerWidth,
        ),
      ).toBe(true)
      const nav = page.getByRole('navigation', { name: '设置导航' })
      const active = nav.getByRole('link', { name: title, exact: true })
      await expect(active).toBeInViewport({ ratio: 1 })
      if (width === 375 && ['plugins', 'skills'].includes(path)) {
        await expect(page.getByRole('article').first()).toBeVisible()
        await page.screenshot({
          path: info.outputPath(path + '-mobile.png'),
          fullPage: true,
        })
      }
    }
  })
}

test('members approve applications and preserve disabled records', async ({
  page,
}) => {
  await login(page, '/settings/members')
  await page.getByRole('button', { name: /加入申请/ }).click()
  await page.getByRole('button', { name: '批准', exact: true }).click()
  await expect(
    page.getByRole('button', { name: '批准', exact: true }),
  ).toHaveCount(0)
  await page.getByRole('button', { name: '成员列表', exact: true }).click()
  const row = page.getByRole('row', { name: /许安/ })
  await row.getByRole('button', { name: '加入历史' }).click()
  await expect(page.getByRole('dialog')).toContainText('申请批准')
  await page.getByRole('button', { name: '关闭详情' }).click()
  await row.getByRole('button', { name: '停用', exact: true }).click()
  await page
    .getByRole('alertdialog')
    .getByRole('button', { name: '停用成员' })
    .click()
  await expect(row.getByText('已停用')).toBeVisible()
  await page.reload()
  await expect(row.getByText('已停用')).toBeVisible()
})

test('model metadata and simulated credentials never send model requests', async ({
  page,
}) => {
  const modelRequests: string[] = []
  page.on('request', (request) => {
    if (/model\.example\.com|\/api\/.*model-/.test(request.url()))
      modelRequests.push(request.url())
  })
  await login(page, '/settings/model-connections')
  await page.getByRole('button', { name: '新建连接' }).click()
  await page.getByLabel('连接名称', { exact: true }).fill('浏览器验收连接')
  await page
    .getByLabel('服务地址', { exact: true })
    .fill('https://model.example.com/v1')
  await page.getByLabel('模型标识', { exact: true }).fill('org/model')
  await page.getByLabel('显示名称', { exact: true }).fill('验收模型')
  await page.getByRole('button', { name: '保存连接', exact: true }).click()
  const row = page.getByRole('article', { name: '浏览器验收连接' })
  await row.getByRole('button', { name: '模拟配置凭据' }).click()
  await expect(row.getByText('演示凭据已配置', { exact: true })).toBeVisible()
  await page
    .getByLabel('默认模型', { exact: true })
    .selectOption({ label: '浏览器验收连接 / 验收模型' })
  await page.getByRole('button', { name: '保存默认模型' }).click()
  await expect(
    page.getByRole('button', { name: '保存默认模型' }),
  ).toBeDisabled()
  await page.reload()
  await expect(page.getByLabel('默认模型', { exact: true })).toHaveValue(
    /org\/model/,
  )
  expect(modelRequests).toEqual([])
  await expect(page.locator('input[type="password"]')).toHaveCount(0)
})
