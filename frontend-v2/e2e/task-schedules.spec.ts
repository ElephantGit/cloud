/// <reference lib="dom" />
import { expect, test } from '@playwright/test'
import { ownerName, signIn } from './login.ts'

const agentsAvailable = !!process.env.V2_NODE_ASSETS_DIR

test('workflows are offered, personal collaboration tasks can be selected, real Fixture model is selectable, and no sample project is presented', async ({ page }) => {
  await page.goto('/schedules')
  await signIn(page)
  await page.locator('header').getByRole('button', { name: '新建任务', exact: true }).click()
  const dialog = page.getByRole('dialog')
  await expect(dialog.getByRole('button', { name: /运行工作流/ })).toBeEnabled()
  await expect(dialog.getByRole('button', { name: /处理协作任务/ })).toBeEnabled()
  // The assembled runtime exposes the credential-free Fixture model and seeded Demo project,
  // while CI without Node assets keeps the Agent list empty.
  const modelSelect = dialog.getByRole('combobox', { name: '模型', exact: true })
  const agentSelect = dialog.getByRole('combobox', { name: 'Agent', exact: true })
  await expect(modelSelect).toBeEnabled()
  await expect(agentSelect).toBeDisabled()
  await expect(dialog.getByRole('option', { name: 'Fixture', exact: true })).toHaveCount(1)
  await expect(dialog.getByRole('option', { name: 'Ora 官网', exact: true })).toHaveCount(0)
  await expect(dialog.getByRole('option', { name: '产品研发', exact: true })).toHaveCount(0)
  await expect(page.getByRole('button', { name: '模拟运行', exact: true })).toHaveCount(0)
  await dialog.getByRole('combobox', { name: '关联项目', exact: true }).selectOption({ label: 'Demo' })
  await expect(agentSelect).toBeEnabled()
  await expect(modelSelect).toBeDisabled()
  if (agentsAvailable) {
    await expect(agentSelect).toContainText('OpenCode')
    await agentSelect.selectOption({ label: 'OpenCode' })
    await expect(modelSelect).toBeEnabled()
    await expect(modelSelect).toContainText('Fixture')
  } else {
    await expect(agentSelect).toContainText('暂无可用 Agent')
  }
  await dialog.getByRole('textbox', { name: '任务名称' }).fill('未保存的输入')
  await page.keyboard.press('Escape')
  await expect(dialog.getByText('放弃未保存的修改？')).toBeVisible()
  await dialog.getByRole('button', { name: '继续编辑' }).click()
  await page.setViewportSize({ width: 390, height: 844 })
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await page.screenshot({ path: test.info().outputPath('schedule-form-resources-mobile.png'), fullPage: true })
})

test('a scheduled personal task opens the exact run on its original collaboration task', async ({page})=>{
  test.setTimeout(360_000)
  const title='计划验收-'+crypto.randomUUID().slice(0,8)
  await page.goto('/collaboration?new=1')
  await signIn(page)
  const dialog=page.getByRole('dialog')
  await dialog.getByLabel('任务标题').fill(title)
  await expect(dialog.getByLabel('所属项目')).not.toHaveValue('')
  await dialog.getByRole('button',{name:'创建任务'}).click()
  await expect(page).toHaveURL(/collaboration\/tasks\//)
  const taskId=new URL(page.url()).pathname.split('/').at(-1)!
  const side=page.getByRole('complementary',{name:'任务属性'})
  await side.getByLabel('指派给').selectOption({label:ownerName})
  await expect(side.getByText('当前：'+ownerName+'（人）')).toBeVisible()
  await page.goto('/schedules')
  await page.locator('header').getByRole('button',{name:'新建任务',exact:true}).click()
  await dialog.getByRole('button',{name:/处理协作任务/}).click()
  await dialog.getByLabel('搜索分配给我的任务').fill(title)
  await dialog.getByLabel('协作任务',{exact:true}).selectOption(taskId)
  await expect(dialog.getByLabel('关联项目')).toBeDisabled()
  await expect(dialog.getByLabel('重复频率')).toHaveValue('once')
  await expect(dialog.getByLabel('任务名称')).toHaveValue(title)
  const executor=dialog.getByLabel('Agent 与模型')
  if(!agentsAvailable) {
    await expect(executor).toContainText('没有可用执行器')
    await dialog.getByRole('button',{name:'创建并启用'}).click()
    await expect(dialog.getByText('请选择 Agent 与模型',{exact:true})).toBeVisible()
    return
  }
  await executor.selectOption({label:'OpenCode / Fixture'})
  await dialog.getByLabel('补充说明').fill('检查项目的 README。')
  await page.screenshot({path:test.info().outputPath('personal-schedule.png'),fullPage:true})
  await dialog.getByRole('button',{name:'创建并启用'}).click()
  await expect(dialog.getByText('OpenCode',{exact:true})).toBeVisible()
  await expect(dialog.getByText('Fixture',{exact:true})).toBeVisible()
  await dialog.getByRole('button',{name:'立即运行'}).click()
  const link=dialog.getByRole('link',{name:'查看本次执行'})
  await expect(link).toBeVisible({timeout:180_000})
  const href=(await link.getAttribute('href'))!
  expect(href).toContain('/collaboration/tasks/'+taskId+'?run=')
  await page.screenshot({path:test.info().outputPath('personal-schedule-history.png'),fullPage:true})
  await link.click()
  await expect(page.getByLabel('运行概况')).toContainText('定时计划触发')
  await expect(page.getByRole('region',{name:'运行与讨论'}).getByText('V2 fixture complete. Real Agent executed the shell tool.')).toBeVisible({timeout:180_000})
  await page.screenshot({path:test.info().outputPath('personal-schedule-result.png'),fullPage:true})
  await page.reload()
  await expect(page.getByLabel('运行概况')).toContainText('定时计划触发')
  await page.goto('/schedules')
  await page.getByRole('button',{name:'查看'+title,exact:true}).click()
  await dialog.getByRole('button',{name:'运行历史'}).click()
  await expect(dialog.getByRole('button',{name:/成功/})).toBeVisible({timeout:30_000})
  await expect(dialog.getByRole('link',{name:'查看本次执行'})).toHaveAttribute('href',href)
  await page.screenshot({path:test.info().outputPath('personal-schedule-history.png'),fullPage:true})
})
