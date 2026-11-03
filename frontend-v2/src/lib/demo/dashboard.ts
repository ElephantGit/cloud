/**
 * Demo data for the dashboard, used only when `VITE_DEMO_MOCK=1` (see `flags.ts`).
 *
 * The page reads one complete `DashboardData`, so the real source can replace this module without
 * touching the page. The real source is not here yet: the task and squad clients live inside
 * `features/collaboration/`, and importing them from another feature directory would couple the
 * two, while moving them to `src/lib/` is a shared change that belongs to the task-workspace work.
 * Replace this file in the same change that adds a dashboard aggregation to the contract.
 *
 * The numbers are internally consistent — member rows add up to the overview tiles, the tiles' notes
 * count the members they describe, the trend adds up to the week's completions — and
 * `dashboard.test.ts` holds the dataset to that, so the recording cannot show two panels that
 * disagree.
 */

/** One overview tile: a short label, its value, and what the number covers. */
export interface DashboardStat {
  label: string
  value: string
  note: string
}

/** One row of team progress; humans and agents share the shape and differ by `kind`. */
export interface TeamMemberProgress {
  id: string
  name: string
  kind: 'human' | 'agent'
  role: string
  /** Tasks the member is working on right now. */
  running: number
  /** Tasks waiting for acceptance. */
  review: number
  /** Tasks finished this week. */
  doneThisWeek: number
}

/** One finished agent run, as the lead sees it in the agent-output list. */
export interface AgentOutput {
  id: string
  agent: string
  task: string
  summary: string
  finished: string
  minutes: number
  status: 'review' | 'done'
}

/** Completions per day, oldest first; the page scales bars against the largest value. */
export interface DailyDone {
  day: string
  done: number
}

/** Everything the dashboard renders. */
export interface DashboardData {
  period: string
  stats: DashboardStat[]
  members: TeamMemberProgress[]
  outputs: AgentOutput[]
  trend: DailyDone[]
}

/** The Tech Lead segment's dashboard: this week of a squad that mixes people and agents. */
export const dashboardDemo: DashboardData = {
  period: '本周 · 11-02 至 11-06',
  stats: [
    { label: '进行中', value: '6', note: '2 人在处理，2 个 Agent 在运行' },
    { label: '待验收', value: '5', note: '其中 2 项由 Agent 提交' },
    { label: '本周完成', value: '12', note: '比上周多 3 项' },
    { label: 'Agent 运行', value: '23 次', note: '18 次已交付' },
  ],
  members: [
    { id: 'member-lead', name: '陈默', kind: 'human', role: 'Tech Lead', running: 1, review: 2, doneThisWeek: 3 },
    { id: 'member-su', name: '苏芮', kind: 'human', role: '研发', running: 2, review: 1, doneThisWeek: 3 },
    { id: 'member-zhou', name: '周衡', kind: 'human', role: '研发', running: 0, review: 0, doneThisWeek: 2 },
    { id: 'member-opencode', name: 'OpenCode', kind: 'agent', role: '执行 Agent', running: 1, review: 2, doneThisWeek: 3 },
    { id: 'member-reviewer', name: 'Reviewer', kind: 'agent', role: '评审 Agent', running: 2, review: 0, doneThisWeek: 1 },
  ],
  outputs: [
    {
      id: 'output-1',
      agent: 'OpenCode',
      task: '子任务表单的校验与错误提示',
      summary: '补齐 3 处字段校验和对应单测，改动 4 个文件。',
      finished: '今天 10:42',
      minutes: 8,
      status: 'review',
    },
    {
      id: 'output-2',
      agent: 'OpenCode',
      task: '看板拖动后的状态回写',
      summary: '接入状态推进接口，处理 409 冲突并回滚卡片位置。',
      finished: '今天 09:15',
      minutes: 15,
      status: 'review',
    },
    {
      id: 'output-3',
      agent: 'Reviewer',
      task: '评审迁移脚本 0074',
      summary: '指出一处缺少回滚路径的 DDL，已提交评论。',
      finished: '昨天 18:40',
      minutes: 6,
      status: 'done',
    },
    {
      id: 'output-4',
      agent: 'OpenCode',
      task: '需求拆分结果的子任务落库',
      summary: '把拆分输出写成子任务并回填来源，改动 store 与路由。',
      finished: '昨天 16:02',
      minutes: 12,
      status: 'done',
    },
  ],
  trend: [
    { day: '11-02', done: 3 },
    { day: '11-03', done: 4 },
    { day: '11-04', done: 2 },
    { day: '11-05', done: 3 },
    { day: '11-06', done: 0 },
  ],
}
