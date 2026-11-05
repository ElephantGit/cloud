import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogTrigger } from '@/components/ui/dialog'
import { simulateDtsSubmission } from '@/lib/demo/dts'
import { demoMockEnabled } from '@/lib/demo/flags'

/** Presents simulation feedback independently of the real Cloud task outcome. */
export function DtsReceipt({ taskId, title, description, severity, module, runId }: { taskId: string; title: string; description: string; severity: string; module: string; runId: string }) {
  const receipt = useQuery({ queryKey: ['dts-demo', taskId], queryFn: () => simulateDtsSubmission(taskId), enabled: demoMockEnabled(), retry: false, staleTime: Infinity })
  if (!demoMockEnabled()) return null
  if (receipt.isError) return <p role="alert">DTS 模拟提交失败，Cloud 任务已保留。<Button variant="outline" onClick={() => void receipt.refetch()}>重试模拟提交</Button></p>
  if (!receipt.data) return <p role="status">正在生成 DTS 模拟回执…</p>
  return <Dialog><DialogTrigger render={<Button variant="outline" className="mt-3" />}>DTS 演示 · {receipt.data.number}</DialogTrigger>
    <DialogContent><DialogHeader><DialogTitle>DTS 模拟问题单</DialogTitle></DialogHeader>
      <p className="text-sm text-muted-foreground">仅供演示，尚未向 DTS 提交。</p>
      <dl className="grid gap-2 text-sm"><dt>单号</dt><dd>{receipt.data.number}</dd><dt>标题</dt><dd>{title}</dd><dt>问题描述</dt><dd>{description}</dd><dt>严重程度</dt><dd>{severity || '未指定'}</dd><dt>所属模块</dt><dd>{module || '未指定'}</dd><dt>处理人</dt><dd>沿用关联 Cloud 任务的负责人</dd></dl>
      <Link className="underline" to={`/collaboration/tasks/${taskId}`}>打开 Cloud 任务</Link><Link className="underline" to={`/chat?run=${runId}`}>来源巡检报告</Link>
    </DialogContent>
  </Dialog>
}
