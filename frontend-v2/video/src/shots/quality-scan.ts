import type { Shot } from '../record/shot.ts'
import { narrationMs } from '../timeline.ts'

/**
 * Segment 4: a nightly version quality scan. 陈默's schedule runs the 「版本质量扫描」 workflow; the
 * shot triggers it by hand instead of waiting for 02:00, follows the real incremental and full scans,
 * and confirms the three findings in the run's form.
 *
 * Every narration line is followed by a hold of its own length, so the picture stays on the page the
 * line describes and the next action never slides out from under an overlay.
 */
export const qualityScan: Shot = {
  name: 'quality-scan',
  scene: 'lead',
  account: 'chen.mo@demo.ora-space.dev',
  start: '/schedules',
  async ready(page) {
    await page.getByRole('row', { name: /版本质量扫描/ }).waitFor()
  },
  async play(d, page) {
    const opening = '发版前的代码质量检查，陈默不再靠人盯。他把它做成了工作流，挂在每天凌晨两点。'
    d.slide({
      eyebrow: 'Tech Lead',
      title: '版本质量扫描',
      subtitle: '每天凌晨扫一遍，问题逐条确认',
      narration: opening,
    })
    await d.focus('page', 1.25)
    const schedule = page.getByRole('row', { name: /版本质量扫描/ })
    await d.mark(schedule, { label: '每天 02:00' })
    const scheduleLine = '定时任务每天跑一次：增量扫描看这几天的改动，全量扫描看整个仓库。'
    d.say({ narration: scheduleLine })
    await d.hold(narrationMs(scheduleLine))

    await d.click(schedule.getByRole('button', { name: /版本质量扫描/ }).first())
    const dialog = page.getByRole('dialog')
    await dialog.getByRole('button', { name: '立即运行' }).waitFor()
    await d.click(dialog.getByRole('button', { name: '立即运行' }))
    const triggerLine = '不能等到凌晨两点，先手动触发一次。'
    d.say({ narration: triggerLine })
    await d.hold(narrationMs(triggerLine))
    await d.click(dialog.getByRole('button', { name: '运行历史' }))

    const explainLines = [
      '扫描分两步，最后汇总成一份报告。',
      '增量扫描只检查这次变动的文件。',
      '全量扫描补上依赖、测试覆盖这些全局规则。',
      '两步结果合并成一份报告，逐条确认。',
    ]
    await d.explain({
      title: '一次扫描怎么走',
      side: 'right',
      focus: dialog.locator('.sp-history-list'),
      narration: explainLines[0],
      items: [
        { title: '增量扫描', body: '只看 release.json 里的变更文件', narration: explainLines[1] },
        { title: '全量扫描', body: '全部源码与测试覆盖', narration: explainLines[2] },
        { title: '报告与确认', body: '两步结果合并，逐条确认', narration: explainLines[3] },
      ],
    })

    const waiting = dialog.getByRole('button', { name: /手动 · 待确认/ }).first()
    await d.wait(waiting, { rate: 10, timeout: 300_000 })
    const scannedLine = '几十秒后，真实扫描完成，运行停在确认这一步。'
    await d.mark(waiting, { label: '报告已生成，等待确认', narration: scannedLine })
    await d.hold(narrationMs(scannedLine))

    await d.click(waiting)
    await d.click(page.getByRole('link', { name: '查看执行详情' }))

    const form = page.getByRole('region', { name: '报告与问题确认' })
    await d.wait(form, { rate: 4, timeout: 60_000 })
    const report = form.locator('details').first()
    await d.focus(report, 1.2)
    const reportLine = '报告先给出扫描范围，再列出去重后的三项发现。'
    await d.mark(report, { label: '扫描范围与结果', narration: reportLine })
    await d.hold(narrationMs(reportLine))

    const findingsLine = '这次抓到三项：硬编码令牌、eval 用法，还有一个模块缺测试。'
    d.say({ narration: findingsLine })
    const findings = form.locator('article').first()
    await findings.scrollIntoViewIfNeeded()
    await d.hold(Math.max(500, narrationMs(findingsLine) - 600))
    await d.mark(findings, { label: '三项发现' })
    await d.hold(600)

    const decisionLine = '令牌和 eval 确认是问题，交给苏芮和周衡；缺测试这条先不动。'
    d.say({ narration: decisionLine })
    await d.select(page.locator('#src\\:hardcoded-token-decision'), '是问题')
    await d.select(page.locator('#src\\:eval-usage-decision'), '是问题')
    await d.select(page.locator('#test\\:uncovered-module-decision'), '不是问题')
    const assignees = page.getByLabel('指派给')
    await assignees.nth(0).waitFor()
    await d.select(assignees.nth(0), '苏芮')
    await d.select(assignees.nth(1), '周衡')
    await d.type(page.locator('#src\\:hardcoded-token-notes'), '发版前修复，令牌改走部署环境', { rate: 6 })
    await d.click(page.getByRole('button', { name: '提交确认' }))

    const done = form.getByText(/确认完成/)
    await d.wait(done, { rate: 4, timeout: 60_000 })
    await done.scrollIntoViewIfNeeded()
    await d.hold(500)
    const finishLine = '提交以后，这次运行继续走完后续节点，确认结果留在报告里。'
    await d.mark(done, { narration: finishLine })
    await d.hold(narrationMs(finishLine))
  },
}
