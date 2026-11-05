import { useEffect, useState } from 'react'
import { ArrowRight, Check, FileText, ShieldCheck, Workflow } from 'lucide-react'
import { Button } from '@/components/ui/button'

// Replaced by an installed cdase workflow, real Agent execution and persisted review artifacts.
// This scenario never starts an Agent, writes a repository file or claims a test has run.
export const cdaseInput = '评审退款申请接口：识别重复提交和退款失败的风险，给出预防措施与验证清单。'
const steps = ['理解需求与边界', '识别失效模式', '评估风险与措施', '生成评审报告']
const findings = [
  { mode: '重复提交导致重复退款', cause: '请求重试时缺少业务幂等约束', impact: '重复资金支出', priority: '高', action: '以订单与退款单标识建立幂等约束；重复请求返回原结果。', verify: '并发提交同一退款单，确认只产生一次退款。' },
  { mode: '渠道失败后状态不明确', cause: '超时被直接视为退款失败', impact: '重试造成重复处理，或退款遗漏', priority: '高', action: '保留处理中状态；通过渠道查询对账后再推进状态。', verify: '模拟渠道超时和延迟回执，核对最终状态与资金记录。' },
]

/** Interactive, deterministic SFMEA recording fixture. All state belongs to this mount. */
export function CdaseDemo() {
  const [selected, setSelected] = useState(false)
  const [input, setInput] = useState('')
  const [step, setStep] = useState(-1)
  const [report, setReport] = useState(false)
  const finished = step === steps.length
  useEffect(() => {
    if (step < 0 || finished) return
    const timer = setTimeout(() => setStep(step + 1), 1000)
    return () => clearTimeout(timer)
  }, [step, finished])

  return <section className="flex min-w-0 flex-1 flex-col bg-background" aria-label="cdase 研发评审">
    <header className="flex h-14 shrink-0 items-center justify-between border-b px-7">
      <div className="flex items-center gap-3 text-sm"><Workflow className="size-4 text-primary" /><span>团队任务清单</span><span className="text-muted-foreground">/</span><span>cdase · SFMEA 评审</span></div>
      <span className="text-xs text-muted-foreground">林新 · 研发团队</span>
    </header>
    <div className="flex min-h-0 flex-1 overflow-y-auto">
      <aside className="w-56 shrink-0 border-r bg-muted/15 p-5">
        <p className="mb-5 text-xs font-medium tracking-wider text-muted-foreground">团队工作流</p>
        <button className="w-full rounded-xl border border-primary/20 bg-primary/5 p-4 text-left" onClick={() => setSelected(true)} aria-label="选择 cdase SFMEA 评审">
          <ShieldCheck className="mb-3 size-6 text-primary" /><p className="text-sm font-semibold">cdase · SFMEA 评审</p><p className="mt-2 text-xs leading-5 text-muted-foreground">从研发需求出发，提前发现失效风险。</p>
          <span className="mt-4 flex items-center gap-2 text-xs text-primary">使用工作流 <ArrowRight className="size-3" /></span>
        </button>
        <p className="mt-6 text-xs leading-6 text-muted-foreground">需求理解 → 风险识别<br />风险评估 → 评审报告</p>
      </aside>
      <main className="min-w-0 flex-1 px-8 py-7">
        {!selected ? <div className="mx-auto mt-20 max-w-lg"><p className="text-xs font-medium text-primary">团队经验 · 随时复用</p><h1 className="mt-4 text-2xl font-semibold">让研发经验进入每一次任务</h1><p className="mt-4 text-sm leading-7 text-muted-foreground">选择团队沉淀的评审工作流，从需求出发，按统一方法检查风险，得到可跟进的评审结论。</p></div> : <div className="mx-auto max-w-3xl space-y-5">
          <div><p className="text-xs font-medium text-primary">CDASE / SFMEA</p><h1 className="mt-2 text-xl font-semibold">退款申请接口 · 研发风险评审</h1><p className="mt-2 text-sm text-muted-foreground">在实现之前识别失效模式，把预防措施和验证要求写清楚。</p></div>
          <ol className="grid grid-cols-4 gap-2" aria-label="评审步骤">{steps.map((title, i) => <li key={title} className={`rounded-xl border p-3 ${step > i ? 'border-success/30 bg-success/5' : step === i ? 'border-primary bg-primary/5' : 'bg-muted/20'}`}><span className="mb-2 flex size-6 items-center justify-center rounded-full bg-background text-xs">{step > i ? <Check className="size-3.5 text-success" /> : i + 1}</span><p className="text-xs font-medium">{title}</p><p className="mt-1 text-[11px] text-muted-foreground">{step > i ? '已完成' : step === i ? '处理中' : '待开始'}</p></li>)}</ol>
          {step < 0 && <form className="rounded-xl border p-5" onSubmit={e => { e.preventDefault(); if (input.trim() === cdaseInput) setStep(0) }}>
            <label htmlFor="cdase-task" className="text-sm font-medium">评审任务</label>
            <textarea id="cdase-task" className="mt-3 min-h-24 w-full resize-none rounded-lg border bg-background p-3 text-sm leading-6" value={input} onChange={e => setInput(e.target.value)} placeholder="填入示例评审任务" />
            <div className="mt-3 flex items-center justify-between gap-4"><button type="button" className="text-xs text-primary" onClick={() => setInput(cdaseInput)}>填入示例任务</button><Button type="submit" disabled={input.trim() !== cdaseInput}>开始评审 <ArrowRight className="size-4" /></Button></div>
            {input && input.trim() !== cdaseInput && <p role="status" className="mt-3 text-xs text-muted-foreground">此示例仅支持预置退款评审任务，请点击「填入示例任务」。</p>}
          </form>}
          {step >= 0 && <div className="rounded-xl border bg-muted/15 px-5 py-3 text-sm leading-6"><span className="mr-2 text-muted-foreground">任务</span>{input}</div>}
          {step >= 0 && !finished && <div role="status" className="rounded-xl border p-5"><p className="text-sm font-medium">{steps[step]}</p><p className="mt-2 text-sm text-muted-foreground">按照团队检查清单，梳理退款流程的失效风险与控制措施。</p></div>}
          {finished && <section aria-label="评审结果" className="rounded-xl border border-success/25 bg-card p-5">
            <div className="flex items-center gap-3"><ShieldCheck className="size-7 text-success" /><div><h2 className="font-semibold">评审完成 · 建议补齐风险措施后实现</h2><p className="mt-1 text-xs text-muted-foreground">已形成风险清单与待验证项，尚未执行测试。</p></div></div>
            <div className="mt-4 grid grid-cols-3 gap-3">{[['失效风险', '2 项'], ['文件改动', '1 份报告'], ['代码改动', '0']].map(([label, value]) => <div className="rounded-lg bg-muted/40 p-3" key={label}><p className="text-xs text-muted-foreground">{label}</p><p className="mt-1 text-sm font-semibold">{value}</p></div>)}</div>
            <ul className="mt-4 space-y-2 text-sm">{findings.map(f => <li key={f.mode} className="flex items-center justify-between border-b py-2 last:border-0"><span>{f.mode}</span><span className="rounded bg-warning-soft px-2 py-1 text-xs text-warning">{f.priority}优先级</span></li>)}</ul>
            <Button className="mt-4" variant="outline" onClick={() => setReport(!report)}><FileText className="size-4" />{report ? '收起评审报告' : '查看评审报告'}</Button>
          </section>}
          {finished && report && <article aria-label="SFMEA 评审报告" className="rounded-xl border p-5">
            <p className="text-xs text-muted-foreground">reports/sfmea-review.md · 新增</p><h2 className="mt-2 text-lg font-semibold">退款申请接口 SFMEA 评审报告</h2>
            <p className="mt-2 text-sm text-muted-foreground">范围：退款提交、渠道响应与失败恢复。输出为评审建议，不代表整改或测试已完成。</p>
            {findings.map(f => <section key={f.mode} className="mt-5 border-t pt-4"><h3 className="text-sm font-semibold">{f.mode}</h3><dl className="mt-2 grid grid-cols-[64px_1fr] gap-y-2 text-sm leading-6"><dt className="text-muted-foreground">原因</dt><dd>{f.cause}</dd><dt className="text-muted-foreground">影响</dt><dd>{f.impact}</dd><dt className="text-muted-foreground">措施</dt><dd>{f.action}</dd><dt className="text-muted-foreground">待验证</dt><dd>{f.verify}</dd></dl></section>)}
          </article>}
        </div>}
      </main>
    </div>
  </section>
}
