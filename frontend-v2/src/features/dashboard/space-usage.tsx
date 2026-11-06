import { Activity, ArrowDownToLine, ArrowUpFromLine, Wrench } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { spaceUsageDemo } from '@/lib/demo/space-usage'

const number = new Intl.NumberFormat('zh-CN')

/** Presents demo workspace usage separately from the unmodified real session analysis. */
export function SpaceUsageSection() {
  const data = spaceUsageDemo()
  if (!data) return null
  const calls = data.tools.reduce((sum, tool) => sum + tool.count, 0)
  const stats = [
    { label: '输入 Token', value: data.inputTokens, icon: ArrowDownToLine, note: '模型请求的输入用量' },
    { label: '输出 Token', value: data.outputTokens, icon: ArrowUpFromLine, note: '推理明细不重复累加' },
    { label: '模型请求', value: data.requests, icon: Activity, note: '每次请求分别计数' },
    { label: '工具调用', value: calls, icon: Wrench, note: '按工具类别汇总' },
  ]
  return <section id="space-usage" aria-label="空间用量" className="mb-8 scroll-mt-6">
    <header className="db-page-header">
      <div>
        <h2 className="db-section-title">空间用量</h2>
        <p>当前空间 · {data.period}</p>
      </div>
      <Badge variant="outline">演示数据</Badge>
    </header>
    <div className="db-stats" aria-label="空间用量汇总">
      {stats.map(({ label, value, icon: Icon, note }) => <article className="db-stat" key={label} aria-label={label}>
        <span className="db-stat-label flex items-center gap-2"><Icon size={14} />{label}</span>
        <strong className="db-stat-value">{number.format(value)}</strong>
        <span className="db-stat-note">{note}</span>
      </article>)}
    </div>
    <section className="db-panel" aria-label="空间工具调用分布">
      <header className="db-panel-head">
        <h2><Wrench size={16} />工具调用分布</h2>
        <span>同一空间 · 同一统计时段</span>
      </header>
      <ul className="db-usage-tools grid gap-5 sm:grid-cols-2 xl:grid-cols-4">
        {data.tools.map(tool => <li key={tool.kind} className="space-y-2">
          <div className="flex items-baseline justify-between gap-3 text-sm">
            <b className="font-medium">{tool.kind}</b>
            <span className="tabular-nums text-muted-foreground">{tool.count} 次</span>
          </div>
          <div className="db-bar h-2" aria-hidden="true"><i style={{ width: `${calls ? tool.count / calls * 100 : 0}%` }} /></div>
        </li>)}
      </ul>
    </section>
    <p className="db-usage-note text-xs text-muted-foreground">空间汇总为演示样例；下方会话分析展示真实运行记录，两者独立。</p>
  </section>
}
