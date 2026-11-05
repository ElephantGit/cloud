import type { Shot } from '../record/shot.ts'

/** Workspace usage is a labelled demo; the selected detail must have real, known gateway usage. */
export const dashboardUsage: Shot = {
  name: 'dashboard-usage',
  scene: 'lead',
  account: 'chen.mo@demo.ora-space.dev',
  start: '/dashboard',
  async ready(page) {
    await page.getByRole('region', { name: '空间用量', exact: true }).waitFor()
    await page.getByRole('list', { name: 'Agent 会话', exact: true }).waitFor()
    const base = new URL(page.url()).origin
    const response = await page.request.get(`${base}/api/v1/me/tenants`)
    const tenants = await response.json() as { items: { id: string }[] }
    const tenant = tenants.items[0]?.id
    if (!tenant) throw new Error('Dashboard recording requires a workspace.')
    const root = `${base}/api/v2/tenants/${tenant}/dashboard/sessions`
    let after = ''
    let selected = ''
    const wanted = process.env.VIDEO_DASHBOARD_SESSION_ID
    do {
      const response = await page.request.get(root, { params: after ? { after } : {} })
      if (!response.ok()) throw new Error('Cannot read dashboard sessions for recording.')
      const sessions = await response.json() as {
        items: { id: string; title: string; model: string; status: string }[]
        nextCursor: string
      }
      for (const session of sessions.items) {
        if (wanted && session.id !== wanted) continue
        if (session.status !== 'completed' || /fixture|预置分析样例/i.test(`${session.title} ${session.model}`)) continue
        const response = await page.request.get(`${root}/${session.id}/statistics`)
        if (!response.ok()) continue
        const stats = await response.json() as {
          counts: { total: number }
          usage: { inputTokens: { value: number | null }; outputTokens: { value: number | null } }
        }
        if (stats.counts.total > 0 && stats.usage.inputTokens.value !== null && stats.usage.outputTokens.value !== null) {
          selected = session.id
          break
        }
      }
      after = sessions.nextCursor
    } while (!selected && after)
    if (!selected) throw new Error('Run a real Agent first: no completed session has known input/output usage.')
    // Resolve the target before recording without changing application data or choosing a seed sample.
    await page.getByRole('list', { name: 'Agent 会话' }).locator(`a[href$="/${selected}"]`).waitFor()
    await page.evaluate(id => { sessionStorage.setItem('video-dashboard-session', id) }, selected)
    console.log(`Dashboard recording uses real session ${selected}`)
  },
  async play(d, page) {
    d.slide({
      eyebrow: '05 / Dashboard',
      title: '从空间用量，到每次执行',
      subtitle: '用量有概览，执行有记录',
      narration: '从空间用量，到每次执行。',
    })
    await d.focus('page', 1.15)
    const usage = page.getByRole('region', { name: '空间用量', exact: true })
    await d.mark(usage.getByLabel('空间用量汇总'), {
      label: '空间用量 · 演示数据',
      narration: '在空间里，集中查看 Token 用量与工具调用。这里展示的是演示数据。',
    })
    await d.hold(7100)

    d.say({ narration: '再进入一次真实运行，查看具体的执行记录。' })
    await d.click(page.getByRole('navigation', { name: 'Dashboard 分区' }).getByRole('link', { name: /会话分析/ }))
    const id = await page.evaluate(() => sessionStorage.getItem('video-dashboard-session'))
    await d.click(page.getByRole('list', { name: 'Agent 会话' }).locator(`a[href$="/${id}"]`))
    await page.getByRole('region', { name: '会话概览' }).waitFor()
    await d.hold(1600)

    await d.click(page.getByRole('button', { name: '模型请求', exact: true }))
    const summary = page.getByRole('region', { name: '模型请求汇总' })
    await summary.waitFor()
    await d.mark(summary.getByText('已记录请求', { exact: true }).locator('..'), {
      label: '真实模型请求',
      narration: '模型发起了多少请求，输入、输出用了多少 Token，都能展开查看。',
    })
    await d.hold(2300)
    const input = summary.getByText('输入 Token', { exact: true }).locator('..')
    await input.scrollIntoViewIfNeeded()
    await d.focus(input, 1.15)
    await d.mark(input, { label: '上游报告的用量', ms: 4000 })
    await d.hold(4100)

    d.say({ narration: '工具调用也有记录：次数、执行状态，以及已记录的耗时。' })
    await d.click(page.getByRole('button', { name: '工具统计', exact: true }))
    const tools = page.getByRole('region', { name: '工具统计', exact: true })
    const row = tools.locator('tbody tr').first()
    await row.waitFor()
    await d.mark(row.locator('td').nth(1), { label: '真实工具记录', ms: 5300 })
    await d.hold(5100)

    d.say({ narration: '从整体用量到单次执行，让 Agent 的工作过程看得见。' })
    await d.click(page.getByRole('button', { name: '时间轴', exact: true }))
    await page.getByRole('region', { name: '会话时间轴', exact: true }).waitFor()
    await d.hold(5100)
  },
}
