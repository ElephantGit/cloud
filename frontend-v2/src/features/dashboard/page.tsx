import { Bot, CheckCircle2, ChevronRight, ClipboardCheck, FlaskConical, PlayCircle, TrendingUp, Users } from 'lucide-react'
import { Placeholder } from '@/app/placeholder'
import { Badge } from '@/components/ui/badge'
import { dashboardDemo, type AgentOutput, type DashboardData, type TeamMemberProgress } from '@/lib/demo/dashboard'
import { demoMockEnabled } from '@/lib/demo/flags'
import { barHeight, completionRate, percent, teamTotals, trendMax } from './dashboard-model'
import './dashboard.css'

/**
 * Team progress and agent output.
 *
 * Without `VITE_DEMO_MOCK=1` the page keeps its placeholder: the aggregation contract does not
 * exist yet, and the mock must never stand in for real statistics outside a demo run. The demo
 * dataset is registered in `src/lib/demo/dashboard.ts`.
 */
export default function Page() {
  if (!demoMockEnabled()) {
    return <Placeholder title="仪表盘" description="团队和 Agent 的运行概况。" />
  }
  return <Dashboard data={dashboardDemo} />
}

function Dashboard({ data }: { data: DashboardData }) {
  const totals = teamTotals(data.members)
  const max = trendMax(data.trend)
  return <div className="dashboard-feature db-page">
    <div className="db-workspace-bar">
      <span className="db-breadcrumb">工作空间<ChevronRight size={13} />仪表盘</span>
      <Badge variant="outline"><FlaskConical data-icon="inline-start" />演示数据</Badge>
    </div>
    <div className="db-content">
      <header className="db-page-header">
        <div>
          <h1>仪表盘</h1>
          <p>团队进度与 Agent 产出 · {data.period}</p>
        </div>
        <div className="db-head-counts" aria-label="团队概况">
          <span><Users size={14} />{totals.humans} 人</span>
          <span><Bot size={14} />{totals.agents} 个 Agent</span>
          <span><PlayCircle size={14} />{totals.running} 项进行中</span>
        </div>
      </header>

      <section className="db-stats" aria-label="本周概览">
        {data.stats.map(stat => <article className="db-stat" key={stat.label}>
          <span className="db-stat-label">{stat.label}</span>
          <strong className="db-stat-value">{stat.value}</strong>
          <span className="db-stat-note">{stat.note}</span>
        </article>)}
      </section>

      <div className="db-panels">
        <section className="db-panel" aria-label="团队进度">
          <header className="db-panel-head">
            <h2><Users size={16} />团队进度</h2>
            <span>按本周分配的任务计算完成率</span>
          </header>
          <ul className="db-members">
            {data.members.map(member => <MemberRow key={member.id} member={member} />)}
          </ul>
        </section>

        <section className="db-panel" aria-label="Agent 产出">
          <header className="db-panel-head">
            <h2><Bot size={16} />Agent 产出</h2>
            <span>最近交付与待验收的结果</span>
          </header>
          <ul className="db-outputs">
            {data.outputs.map(output => <OutputRow key={output.id} output={output} />)}
          </ul>
        </section>
      </div>

      <section className="db-panel db-trend" aria-label="本周完成趋势">
        <header className="db-panel-head">
          <h2><TrendingUp size={16} />本周完成趋势</h2>
          <span>每天进入「已完成」的任务数</span>
        </header>
        <ol className="db-bars">
          {data.trend.map(day => <li key={day.day}>
            <span className="db-bar-track" aria-hidden="true">
              <i style={{ height: percent(barHeight(day, max)) }} />
            </span>
            <b>{day.done}</b>
            <small>{day.day}</small>
          </li>)}
        </ol>
      </section>

      <p className="db-note">统计来自演示数据，真实统计待任务聚合接口接入后替换；任务执行本身仍是真实运行。</p>
    </div>
  </div>
}

function MemberRow({ member }: { member: TeamMemberProgress }) {
  const rate = completionRate(member)
  return <li className={`db-member ${member.kind}`}>
    <span className="db-member-name">
      {member.kind === 'agent' ? <Bot size={15} /> : <Users size={15} />}
      <b>{member.name}</b>
      <em>{member.role}</em>
    </span>
    <span className="db-member-counts">
      <span><PlayCircle size={13} />进行中 {member.running}</span>
      <span><ClipboardCheck size={13} />待验收 {member.review}</span>
      <span><CheckCircle2 size={13} />本周完成 {member.doneThisWeek}</span>
    </span>
    <span className="db-member-rate">
      <span className="db-bar" role="progressbar" aria-label={`${member.name} 完成率`} aria-valuenow={Math.round(rate * 100)} aria-valuemin={0} aria-valuemax={100}>
        <i style={{ width: percent(rate) }} />
      </span>
      <b>{percent(rate)}</b>
    </span>
  </li>
}

function OutputRow({ output }: { output: AgentOutput }) {
  return <li className="db-output">
    <span className="db-output-head">
      <b>{output.task}</b>
      <Badge variant={output.status === 'review' ? 'secondary' : 'outline'}>{output.status === 'review' ? '待验收' : '已交付'}</Badge>
    </span>
    <span className="db-output-summary">{output.summary}</span>
    <span className="db-output-meta">{output.agent} · {output.minutes} 分钟 · {output.finished}</span>
  </li>
}
