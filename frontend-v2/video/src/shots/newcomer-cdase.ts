import type { Shot } from '../record/shot.ts'

const task = '评审退款申请接口：识别重复提交和退款失败的风险，给出预防措施与验证清单。'

/** Newcomer chapter: a deterministic frontend SFMEA scenario, not a live model run. */
export const newcomerCdase: Shot = {
  name: 'newcomer-cdase',
  scene: 'newcomer',
  account: 'lin.xin@demo.ora-space.dev',
  start: '/chat?demo=cdase',
  async ready(page) {
    await page.getByRole('heading', { name: '让研发经验进入每一次任务' }).waitFor()
  },
  async play(d, page) {
    d.slide({
      eyebrow: '02 / 研发经验',
      title: '研发经验实际固化成工作流',
      subtitle: '提升日常工作效率和质量',
      narration: '研发经验实际固化成工作流，提升日常工作效率和质量。',
    })
    d.say({ narration: '新人接到任务，可以直接使用团队沉淀的研发流程。' })
    await d.focus('page', 1.15)
    await d.mark(page.getByRole('button', { name: '选择 cdase SFMEA 评审' }), { label: '团队经验 · 随时复用' })
    await d.hold(1800)
    await d.click(page.getByRole('button', { name: '选择 cdase SFMEA 评审' }))
    d.say({ narration: '从理解需求到风险评审，团队的方法已经组织成清晰的步骤。' })
    await d.mark(page.getByRole('list', { name: '评审步骤' }), { label: '统一方法 · 四步评审' })
    await d.hold(1800)

    d.say({ narration: '输入需求，检查重复提交和失败恢复的风险。' })
    await d.type(page.getByLabel('评审任务'), task, { rate: 3 })
    await d.hold(500)
    await d.click(page.getByRole('button', { name: '开始评审' }))
    await d.explain({
      title: '把研发方法变成可重复的步骤',
      focus: page.getByRole('list', { name: '评审步骤' }),
      zoom: 1.1,
      items: [
        { title: '明确边界', body: '需求、业务流程与异常场景', narration: '先明确需求边界。' },
        { title: '识别失效', body: '从失效模式追溯原因与影响', narration: '再识别失效模式，追溯原因与影响。' },
        { title: '落实措施', body: '预防措施与待验证清单', narration: '最后把预防措施和验证要求写清楚。' },
      ],
    })
    const result = page.getByRole('region', { name: '评审结果' })
    await d.wait(result, { rate: 1 })
    await d.mark(result, { label: '风险清单 · 具体建议', narration: '评审完成后，新人拿到风险清单和具体建议。' })
    await d.hold(1800)
    await d.click(page.getByRole('button', { name: '查看评审报告' }))
    const report = page.getByRole('article', { name: 'SFMEA 评审报告' })
    await report.scrollIntoViewIfNeeded()
    d.say({ narration: '每项风险都能追溯到原因、措施和待验证项。' })
    await d.mark(report, { label: '评审报告 · 可追溯' })
    await d.hold(2400)
    d.say({ narration: '经验进入每一次任务，帮助新人做得更稳。' })
    await d.hold(4200)
  },
}
