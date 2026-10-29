import { expect, test } from '@playwright/test'

test('workflows: open a sample, build a chain, keep it across a reload, publish', async ({ page }) => {
  await page.goto('/workflows')
  await page.getByRole('button', { name: '开发者登录' }).click()
  const library = page.getByRole('complementary', { name: '工作流列表' })

  // A fresh browser finds the published samples, the first one open on the canvas.
  await expect(library.getByRole('button', { name: '示例：代码审查', exact: true })).toBeVisible()
  await expect(library.getByRole('button', { name: '循环综合测试：分支与退出', exact: true })).toBeVisible()
  await expect(page.getByText('生效中 · v1')).toBeVisible()

  // Create a workflow and add nodes from the catalog.
  await library.getByRole('button', { name: '新建或导入工作流' }).click()
  await page.getByRole('menuitem', { name: /新建工作流/ }).click()
  const create = page.getByRole('alertdialog', { name: '新建工作流' })
  await create.getByLabel('工作流名称').fill('端到端验收')
  await create.getByRole('button', { name: '新建工作流' }).click()
  await expect(create).toBeHidden()
  const canvas = page.getByLabel('工作流画布')
  await expect(page.getByRole('textbox', { name: '工作流名称' })).toHaveValue('端到端验收')
  const catalog = page.getByRole('toolbar', { name: '添加工作流节点' })
  for (const kind of ['Agent', '输出']) {
    await catalog.getByRole('button', { name: kind, exact: true }).click()
    // Adding a node opens its inspector over the catalog; close it before the next one.
    await canvas.press('Escape')
  }
  await expect(canvas).toHaveAttribute('data-workflow-node-count', '3')

  // The draft autosaves, so a reload restores it.
  await expect(page.getByText('已实时保存')).toBeVisible()
  await page.reload()
  await expect(page.getByRole('textbox', { name: '工作流名称' })).toHaveValue('端到端验收')
  await expect(canvas).toHaveAttribute('data-workflow-node-count', '3')

  // Publishing freezes the draft as a named version.
  await expect(page.getByText('暂未发布')).toBeVisible()
  await page.getByRole('button', { name: '发布', exact: true }).click()
  const publish = page.getByRole('alertdialog', { name: '发布工作流' })
  await publish.getByRole('textbox').fill('v1')
  await publish.getByRole('button', { name: '发布' }).click()
  await expect(page.getByText('生效中 · v1')).toBeVisible()

  // Every toast can be dismissed.
  const toast = page.getByText('已发布版本 v1')
  await expect(toast).toBeVisible()
  await page.getByRole('button', { name: 'Close toast' }).click()
  await expect(toast).toBeHidden()
})
