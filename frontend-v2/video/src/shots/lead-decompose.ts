import type { Shot } from '../record/shot.ts'

const title = '看板支持按负责人筛选和批量指派'
const description = [
  '小队人多以后，看板上很难找到自己的任务，Tech Lead 分派也要一张张点。',
  '验收：看板顶部可以按负责人筛选；可以多选卡片一次指派给同一个人或 Agent；筛选条件刷新后保留。',
].join('\n')

/** B（Tech Lead 陈默）：新建一个大需求，用「需求拆分」拆成子任务，子任务出现在看板上。 */
export const leadDecompose: Shot = {
  name: 'lead-decompose',
  scene: 'lead',
  account: 'chen.mo@demo.ora-space.dev',
  start: '/collaboration',
  async ready(page) {
    await page.getByRole('list', { name: '任务看板' }).waitFor()
  },
  async play(d, page) {
    d.say({
      caption: '陈默 · Tech Lead',
      narration: '陈默带着一支小队：三个人，加上两个 Agent。大需求进来，他不再自己一条条拆。',
    })
    await d.focus('page')
    await d.hold(1800)

    d.say({ caption: '新建一个大需求', narration: '先把需求原样写下来。' })
    await d.click(page.getByRole('button', { name: '新建任务' }).first())
    const dialog = page.getByRole('dialog')
    await d.type(dialog.getByLabel('任务标题'), title)
    await d.type(dialog.getByLabel('描述（可选）'), description, { rate: 6 })
    await d.focus(dialog)
    await d.hold(800)
    await d.click(dialog.getByRole('button', { name: '创建任务' }))

    d.say({ caption: '一键拆分', narration: '点「拆分」，交给团队的「需求拆分」工作流。' })
    await page.getByRole('heading', { level: 2, name: title }).waitFor()
    await d.focus('page')
    await d.hold(900)
    await d.click(page.getByRole('button', { name: '拆分', exact: true }))
    const split = page.getByRole('dialog')
    await d.focus(split)
    await d.hold(1200)
    await d.click(split.getByRole('button', { name: '开始拆分' }))

    d.say({ caption: '工作流读需求、拆任务', narration: '工作流读完需求，按可以独立交付的粒度拆成子任务。' })
    const subtasks = page.getByRole('list', { name: '子任务' })
    await d.wait(subtasks, { camera: page.getByRole('region', { name: '运行与讨论' }), rate: 10 })
    await d.focus(subtasks)
    d.say({ caption: '子任务已建好，等待指派', narration: '几十秒后，子任务都建好了，每一条都写清了要做什么。' })
    await d.hold(2600)

    d.say({ caption: '回到看板', narration: '回到看板，它们已经排在「待处理」里，等着分给人或 Agent。' })
    await d.click(page.getByRole('link', { name: '返回任务列表' }))
    const pending = page.getByRole('listitem', { name: '待处理列' })
    await pending.waitFor()
    await d.focus('page')
    await d.hold(900)
    await d.focus(pending)
    await d.hold(2400)
    await d.focus('page')
    d.say({ caption: '' })
    await d.hold(1500)
  },
}
