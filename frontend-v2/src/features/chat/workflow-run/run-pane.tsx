import { ChevronRightIcon, FlaskConicalIcon, PlayIcon, RotateCcwIcon, SquareIcon, WorkflowIcon } from 'lucide-react'
import { useState } from 'react'
import { I18nextProvider } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { subtleScrollbar } from '@/lib/scrollbar'
import { cn } from '@/lib/utils'
import { createWorkflowI18n } from '@/lib/workflows/host/i18n'
import type { GraphWorkflowNodeState, GraphWorkflowRun, WorkflowNodeConversationItem } from '@/lib/workflows/runtime'
import { FadeText } from '../components/fade-text'
import { Markdown } from '../components/markdown'
import { ShowSidebarButton } from '../components/sidebar'
import { useChatStore } from '../store'
import { cancelDemoRun, rerunDemoRun, startDemoRun, useDemoRuns } from './demo-runs'
import { RunOverviewCanvas } from './run-overview-canvas'
import { isNodeWorking, runStatusLabel, runStatusTone } from './run-status-style'
import { formatRunClock } from './run-format'
import { workflowRunTranslations } from './translations'
import { WorkflowRunStartDialog } from './workflow-run-start-dialog'

/** The workflow bundles plus the run view's strings, for the components copied from desktop. */
const runI18n = createWorkflowI18n(['zh-CN'], {
  'zh-CN': {
    ...workflowRunTranslations['zh-CN'],
    // Desktop's hint sends a click to the Theater view, which this page does not have.
    'workflowRun.overview.hint': '点击节点，在右侧查看它的输入、输出和对话',
  },
})

const noItems: WorkflowNodeConversationItem[] = []
const terminal = new Set<GraphWorkflowRun['status']>(['succeeded', 'failed', 'cancelled'])

/**
 * A workflow run in the chat page's main area, as desktop's run workspace: the frozen graph with
 * each node's status, and the picked node's input, output and Agent conversation. The run is
 * played by the in-browser engine, so the header says it is a demo.
 */
export default function RunPane({ runId }: { runId: string }) {
  const run = useDemoRuns((s) => s.runs[runId])
  if (!run) {
    return (
      <div className="flex flex-1 items-center justify-center text-sm text-muted-foreground">
        <p>这次运行已经不在了。</p>
      </div>
    )
  }
  return (
    <I18nextProvider i18n={runI18n}>
      <RunWorkspace run={run} />
    </I18nextProvider>
  )
}

function RunWorkspace({ run }: { run: GraphWorkflowRun }) {
  const conversation = useDemoRuns((s) => s.conversations[run.id] ?? noItems)
  const selectRun = useChatStore((s) => s.selectRun)
  const [picked, setPicked] = useState<string | null>(null)
  const [startOpen, setStartOpen] = useState(false)
  const [busy, setBusy] = useState(false)
  const nodes = run.definitionSnapshot.nodes
  const startNode = nodes.find((n) => n.data.kind === 'start')
  // Follows the node at work until the person picks one, as desktop's run view does.
  const working = nodes.find((n) => isNodeWorking(run.nodeStates[n.id]?.status ?? 'idle'))
  const focusedId = picked ?? working?.id ?? (terminal.has(run.status) ? lastFinished(run) : startNode?.id) ?? null
  const focused = nodes.find((n) => n.id === focusedId)
  const tone = runStatusTone(run.status)
  const finished = terminal.has(run.status)

  const act = async (action: () => Promise<void>) => {
    setBusy(true)
    try {
      await action()
    } finally {
      setBusy(false)
    }
  }

  return (
    <>
      <header className="flex h-12 shrink-0 items-center gap-2 border-b px-4">
        <ShowSidebarButton />
        <WorkflowIcon className="size-4 shrink-0 text-muted-foreground" />
        <h2 className="flex min-w-0 text-sm font-medium">
          <FadeText text={run.name} />
        </h2>
        <Badge variant="outline" className={cn('gap-1.5', tone.badge)}>
          <span className={cn('size-1.5 rounded-full', tone.dot)} />
          {runStatusLabel(run.status)}
        </Badge>
        <Badge variant="outline" className="gap-1 text-muted-foreground" title="后端还不能执行工作流，节点由浏览器里的演示引擎按顺序推进">
          <FlaskConicalIcon />
          演示运行
        </Badge>
        <div className="ml-auto flex shrink-0 items-center gap-1">
          {run.status === 'pending' && (
            <Button size="sm" onClick={() => setStartOpen(true)} disabled={busy}>
              <PlayIcon />
              启动
            </Button>
          )}
          {(run.status === 'running' || run.status === 'awaiting_input') && (
            <Button variant="outline" size="sm" onClick={() => act(() => cancelDemoRun(run.id))} disabled={busy}>
              <SquareIcon />
              停止
            </Button>
          )}
          {finished && (
            <Button
              variant="outline"
              size="sm"
              disabled={busy}
              onClick={() =>
                act(async () => {
                  const next = await rerunDemoRun(run.id)
                  selectRun(next.id)
                })
              }
            >
              <RotateCcwIcon />
              再跑一次
            </Button>
          )}
        </div>
      </header>
      <div className="flex min-h-0 flex-1">
        <div className="relative flex min-w-0 flex-1 flex-col">
          {run.status === 'pending' && (
            <p className="absolute inset-x-0 top-3 z-10 mx-auto w-fit rounded-md border bg-background/90 px-3 py-1.5 text-xs text-muted-foreground shadow-sm">
              还没开始：点右上角「启动」，填写输入后运行
            </p>
          )}
          <RunOverviewCanvas run={run} focusedNodeId={focusedId} onFocusNode={setPicked} />
        </div>
        <aside aria-label="节点详情" className={cn('flex w-[340px] shrink-0 flex-col overflow-y-auto border-l', subtleScrollbar)}>
          {finished && <RunResult run={run} />}
          {focused ? (
            <NodeDetails
              title={focused.data.title}
              kind={focused.data.kind}
              state={run.nodeStates[focused.id] ?? { status: 'idle' }}
              items={conversation.filter((item) => item.nodeId === focused.id)}
            />
          ) : (
            <p className="p-4 text-sm text-muted-foreground">在左侧点一个节点查看它的输入、输出和对话。</p>
          )}
        </aside>
      </div>
      <WorkflowRunStartDialog
        open={startOpen}
        runName={run.name}
        initialPrompt={startNode?.data.input ?? ''}
        variables={startNode?.data.inputVariables ?? []}
        busy={busy}
        onOpenChange={setStartOpen}
        onStart={async (prompt, variables) => {
          await act(() => startDemoRun(run.id, prompt, variables))
          setStartOpen(false)
        }}
      />
    </>
  )
}

/** The node that finished last, shown once the run has ended and nothing is picked. */
function lastFinished(run: GraphWorkflowRun): string | undefined {
  return Object.entries(run.nodeStates)
    .filter(([, state]) => state.finishedAt)
    .sort(([, a], [, b]) => (b.finishedAt ?? '').localeCompare(a.finishedAt ?? ''))[0]?.[0]
}

/** What the run ended with: the Output nodes' results, or why it stopped. */
function RunResult({ run }: { run: GraphWorkflowRun }) {
  const outputs = run.definitionSnapshot.nodes.flatMap((n) => {
    const output = n.data.kind === 'output' ? run.nodeStates[n.id]?.output : undefined
    return output ? [{ id: n.id, title: n.data.title, text: output.detail || output.summary }] : []
  })
  return (
    <section aria-label="运行结果" className="border-b bg-muted/30 p-4">
      <h3 className="mb-2 text-xs font-medium text-muted-foreground">运行结果 · {runStatusLabel(run.status)}</h3>
      {run.status === 'cancelled' && <p className="text-sm">运行已停止，后面的节点没有执行。</p>}
      {run.status === 'failed' && <p className="text-sm text-destructive">运行失败，点失败的节点查看原因。</p>}
      {run.status === 'succeeded' && outputs.length === 0 && <p className="text-sm">运行完成。</p>}
      {outputs.map((o) => (
        <div key={o.id} className="text-sm">
          <p className="font-medium">{o.title}</p>
          <Markdown text={o.text} />
        </div>
      ))}
    </section>
  )
}

function NodeDetails({ title, kind, state, items }: { title: string; kind: string; state: GraphWorkflowNodeState; items: WorkflowNodeConversationItem[] }) {
  const tone = runStatusTone(state.status)
  const messages = items.filter((item) => item.kind === 'message')
  const activity = items.filter((item) => item.kind === 'activity')
  return (
    <section aria-label={`节点 ${title}`} className="flex flex-col gap-4 p-4">
      <div className="flex items-start gap-2">
        <div className="min-w-0 flex-1">
          <h3 className="text-sm font-medium break-words">{title}</h3>
          <p className="text-xs text-muted-foreground">
            {kind}
            {state.startedAt && ` · 开始 ${formatRunClock(state.startedAt)}`}
            {state.finishedAt && ` · 结束 ${formatRunClock(state.finishedAt)}`}
          </p>
        </div>
        <Badge variant="outline" className={cn('shrink-0 gap-1.5', tone.badge)}>
          <span className={cn('size-1.5 rounded-full', tone.dot, isNodeWorking(state.status) && 'animate-pulse')} />
          {runStatusLabel(state.status)}
        </Badge>
      </div>
      {state.errorMessage && <p className="rounded-md bg-destructive/10 p-2 text-sm text-destructive">{state.errorMessage}</p>}
      {state.input && <Io label="输入" summary={state.input.summary} detail={state.input.detail} />}
      {state.output && <Io label="输出" summary={state.output.summary} detail={state.output.detail} />}
      {(messages.length > 0 || activity.length > 0) && (
        <div className="flex flex-col gap-2">
          <h4 className="text-xs font-medium text-muted-foreground">节点对话</h4>
          {activity.length > 0 && (
            <Collapsible>
              <CollapsibleTrigger className="group/trigger flex items-center gap-1 text-xs text-muted-foreground">
                <ChevronRightIcon className="size-3 transition-transform group-aria-expanded/trigger:rotate-90" />
                活动 {activity.length} 项
              </CollapsibleTrigger>
              <CollapsibleContent>
                <ul className="mt-1 flex flex-col gap-1 border-l pl-3 text-xs text-muted-foreground">
                  {activity.map((item) => item.kind === 'activity' && <li key={item.id}>{item.summary}</li>)}
                </ul>
              </CollapsibleContent>
            </Collapsible>
          )}
          {messages.map(
            (item) =>
              item.kind === 'message' && (
                <div
                  key={item.id}
                  className={cn('rounded-lg px-3 py-2 text-sm', item.role === 'user' ? 'ml-6 bg-muted' : 'border bg-background')}
                  aria-label={item.role === 'user' ? '发给 Agent 的内容' : 'Agent 回复'}
                >
                  <Markdown text={item.markdown} />
                </div>
              ),
          )}
        </div>
      )}
      {state.status === 'idle' && !state.input && <p className="text-sm text-muted-foreground">这个节点还没执行。</p>}
    </section>
  )
}

function Io({ label, summary, detail }: { label: string; summary: string; detail?: string }) {
  return (
    <div className="flex flex-col gap-1">
      <h4 className="text-xs font-medium text-muted-foreground">{label}</h4>
      <div className="rounded-md border bg-muted/20 px-3 py-2 text-sm">
        <Markdown text={detail?.trim() ? detail : summary} />
      </div>
    </div>
  )
}
