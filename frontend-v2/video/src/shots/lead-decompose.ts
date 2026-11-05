import type { Shot } from '../record/shot.ts'

const title = '看板支持按负责人筛选和批量指派'
const description = [
  '小队人多以后，看板上很难找到自己的任务，Tech Lead 分派也要一张张点。',
  '验收：看板顶部可以按负责人筛选；可以多选卡片一次指派给同一个人或 Agent；筛选条件刷新后保留。',
].join('\n')

/**
 * The worked example of the shot API: a chapter page, quick work at a steady zoom, the window
 * stepping aside over a real run, and a highlight on the result. Tech Lead 陈默 creates a big
 * requirement, splits it with the 需求拆分 workflow, and finds the subtasks on the board.
 */
export const leadDecompose: Shot = {
  name: 'lead-decompose',
  scene: 'lead',
  account: 'chen.mo@demo.ora-space.dev',
  start: '/collaboration',
  async ready(page) {
    await page.getByRole('list', { name: '任务看板' }).waitFor()
  },
  async play(d, page) {
    d.slide({
      eyebrow: 'Tech Lead',
      title: '大需求，交给工作流来拆',
      subtitle: '陈默带着三个人和两个 Agent',
      narration: '陈默带着一支小队：三个人，加上两个 Agent。大需求进来，他不再自己一条条拆。',
    })

    d.say({ narration: '先把需求原样写下来。' })
    await d.focus('page', 1.3)
    await d.click(page.getByRole('button', { name: '新建任务' }).first())
    const dialog = page.getByRole('dialog')
    await d.type(dialog.getByLabel('任务标题'), title)
    await d.select(dialog.getByLabel('所属项目'), '协作看板服务')
    await d.type(dialog.getByLabel('描述（可选）'), description, { rate: 6 })
    await d.hold(600)
    await d.click(dialog.getByRole('button', { name: '创建任务' }))

    d.say({ narration: '点「拆分」，交给团队的「需求拆分」工作流。' })
    await page.getByRole('heading', { level: 2, name: title }).waitFor()
    await d.hold(600)
    await d.click(page.getByRole('button', { name: '拆分', exact: true }))
    const split = page.getByRole('dialog')
    await d.hold(800)
    await d.click(split.getByRole('button', { name: '开始拆分' }))

    await d.explain({
      title: '工作流在做什么',
      narration: '工作流接手以后，做三件事。',
      items: [
        { title: '读需求', body: '标题、描述和验收条件', narration: '先读需求，包括描述和验收条件。' },
        { title: '拆粒度', body: '每一条都能独立交付', narration: '再按可以独立交付的粒度拆开。' },
        { title: '建任务', body: '子任务直接落到看板', narration: '最后建好子任务，直接落到看板上。' },
      ],
    })

    const subtasks = page.getByRole('list', { name: '子任务' })
    await d.wait(subtasks, { rate: 10 })
    await d.mark(subtasks, { label: '拆出来的子任务', narration: '几十秒后，子任务都建好了，每一条都写清了要做什么。' })
    await d.hold(600)

    d.say({ narration: '回到看板，它们已经排在「待处理」里，等着分给人或 Agent。' })
    await d.click(page.getByRole('link', { name: '返回任务列表' }))
    const pending = page.getByRole('listitem', { name: '待处理列' })
    await pending.waitFor()
    await d.focus('page', 1)
    await d.hold(600)
    await d.mark(pending, { label: '待处理' })
    await d.hold(800)
  },
}
