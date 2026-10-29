import type * as acp from '@agentclientprotocol/sdk'
import {
  BotIcon,
  CircleAlertIcon,
  CircleCheckIcon,
  CircleDotIcon,
  CircleIcon,
  CircleSlashIcon,
  ListChecksIcon,
  LoaderCircleIcon,
  RouteIcon,
  TelescopeIcon,
} from 'lucide-react'
import { createContext, useContext, useState } from 'react'
import {
  Trace,
  TraceCollapsible,
  TraceCollapsibleContent,
  TraceCollapsibleTrigger,
  TraceDescription,
  TraceIcon,
  TraceLine,
  TraceTitle,
} from '@/components/ui/trace'
import type { ActivityEntry, ExploreGroup, Row, SubagentGroup } from '../model/display'
import type { PermissionRequest, PlanItem, ThoughtItem, ToolItem, TurnStatus } from '../model/types'
import { formatDuration } from './format'
import { Markdown } from './markdown'
import { ToolRow } from './tool'
import { toolLineStatus } from './tool-status'

type Source = Extract<acp.ContentBlock, { type: 'resource_link' }>

/** What every row of one turn needs to know about the turn around it. */
export interface TurnScope {
  status: TurnStatus
  permissions: PermissionRequest[]
  sources: Source[]
  onAnswer: (permissionId: string, optionId: string) => void
}

const TurnContext = createContext<TurnScope>({ status: 'completed', permissions: [], sources: [], onAnswer: () => {} })

/** Provides a turn's scope to the rows rendered inside it. */
export const TurnProvider = TurnContext.Provider

/** A list of rows: agent text, inline content and activity blocks. Used for turns and subagents. */
export function Rows({ rows }: { rows: Row[] }) {
  const { sources } = useContext(TurnContext)
  return rows.map((row) => {
    switch (row.kind) {
      case 'message':
        return <Markdown key={row.id} text={row.text} sources={sources} />
      case 'content':
        return <InlineContent key={row.id} block={row.block} />
      case 'activity':
        return <ActivityBlock key={row.id} entries={row.entries} />
    }
  })
}

function InlineContent({ block }: { block: Exclude<acp.ContentBlock, { type: 'text' }> }) {
  // Resource links are listed once per turn as sources, not inline.
  if (block.type === 'resource_link') return null
  if (block.type === 'image') {
    return <img src={`data:${block.mimeType};base64,${block.data}`} alt="Agent 发送的图片" className="max-h-80 w-fit rounded-lg border" />
  }
  return <p className="text-xs text-muted-foreground">（暂不支持显示的内容：{block.type}）</p>
}

/**
 * The work between two runs of agent text. While the turn streams every step shows; once it
 * ends, several steps fold into one summary line so the answer stays in view.
 */
function ActivityBlock({ entries }: { entries: ActivityEntry[] }) {
  const { status, permissions } = useContext(TurnContext)
  const live = status === 'streaming'
  const waiting = entries.some((e) => e.kind === 'tool' && permissions.some((p) => p.toolCall.toolCallId === e.id && !p.outcome))
  if (live || waiting || entries.length <= 1) {
    return (
      <Trace>
        {entries.map((e) => (
          <Entry key={e.id} entry={e} />
        ))}
      </Trace>
    )
  }
  const tools = entries.flatMap((e) => (e.kind === 'tool' ? [e] : e.kind === 'explore' ? e.tools : []))
  const failed = tools.some((t) => t.status === 'failed')
  const cancelled = status === 'cancelled' || tools.some((t) => t.status === 'cancelled')
  const count = entries.reduce((n, e) => n + (e.kind === 'explore' ? e.tools.length : 1), 0)
  return (
    <Trace>
      <TraceCollapsible>
        <TraceCollapsibleTrigger>
          <TraceLine status={failed ? 'error' : undefined}>
            <TraceIcon>{failed ? <CircleAlertIcon /> : cancelled ? <CircleSlashIcon /> : <ListChecksIcon />}</TraceIcon>
            <TraceTitle>{failed ? '部分操作失败' : cancelled ? '操作已中断' : '已完成的操作'}</TraceTitle>
            <TraceDescription>{count} 项</TraceDescription>
          </TraceLine>
        </TraceCollapsibleTrigger>
        <TraceCollapsibleContent>
          <Trace>
            {entries.map((e) => (
              <Entry key={e.id} entry={e} />
            ))}
          </Trace>
        </TraceCollapsibleContent>
      </TraceCollapsible>
    </Trace>
  )
}

function Entry({ entry }: { entry: ActivityEntry }) {
  switch (entry.kind) {
    case 'thought':
      return <ThoughtRow thought={entry} />
    case 'plan':
      return <PlanRow plan={entry} />
    case 'explore':
      return <ExploreRow group={entry} />
    case 'subagent':
      return <SubagentRow group={entry} />
    case 'tool':
      return <LiveToolRow tool={entry} subagent={entry.subagent} />
  }
}

function LiveToolRow({ tool, subagent }: { tool: ToolItem; subagent?: SubagentGroup }) {
  const { status, permissions, onAnswer } = useContext(TurnContext)
  const permission = permissions.find((p) => p.toolCall.toolCallId === tool.id)
  return (
    <ToolRow
      tool={tool}
      live={status === 'streaming'}
      permission={permission}
      onAnswer={onAnswer}
      subagent={subagent && subagent.rows.length > 0 ? <Rows rows={subagent.rows} /> : undefined}
    />
  )
}

/** The agent's reasoning: open while it thinks, then one line saying for how long. */
function ThoughtRow({ thought }: { thought: ThoughtItem }) {
  const { status } = useContext(TurnContext)
  const running = thought.endedAt === undefined && status === 'streaming'
  const [open, setOpen] = useState<boolean>()
  const took = thought.endedAt !== undefined ? formatDuration(thought.endedAt - thought.startedAt) : undefined
  return (
    <TraceCollapsible open={open ?? running} onOpenChange={setOpen}>
      <TraceCollapsibleTrigger>
        <TraceLine status={running ? 'running' : undefined}>
          <TraceIcon>{running ? <LoaderCircleIcon className="animate-spin" /> : <CircleCheckIcon />}</TraceIcon>
          <TraceTitle>{running ? '思考中' : took ? `思考了 ${took}` : '思考'}</TraceTitle>
        </TraceLine>
      </TraceCollapsibleTrigger>
      <TraceCollapsibleContent className="whitespace-pre-wrap">{thought.text}</TraceCollapsibleContent>
    </TraceCollapsible>
  )
}

/** ACP's plan: each entry with its status, and how many are done. */
function PlanRow({ plan }: { plan: PlanItem }) {
  const { status } = useContext(TurnContext)
  const live = status === 'streaming'
  const done = plan.entries.filter((e) => e.status === 'completed').length
  return (
    <TraceCollapsible defaultOpen>
      <TraceCollapsibleTrigger>
        <TraceLine status={live && done < plan.entries.length ? 'running' : undefined}>
          <TraceIcon>
            <RouteIcon />
          </TraceIcon>
          <TraceTitle>计划</TraceTitle>
          <TraceDescription>
            {done} / {plan.entries.length} 已完成
          </TraceDescription>
        </TraceLine>
      </TraceCollapsibleTrigger>
      <TraceCollapsibleContent>
        <Trace aria-label="计划步骤">
          {plan.entries.map((e) => (
            <TraceLine key={e.content} status={e.status === 'pending' ? 'pending' : live && e.status === 'in_progress' ? 'running' : undefined}>
              <TraceIcon>
                {e.status === 'completed' ? <CircleCheckIcon /> : e.status === 'in_progress' ? <CircleDotIcon /> : <CircleIcon />}
              </TraceIcon>
              <TraceTitle className={e.status === 'completed' ? 'line-through decoration-muted-foreground/50' : undefined}>{e.content}</TraceTitle>
            </TraceLine>
          ))}
        </Trace>
      </TraceCollapsibleContent>
    </TraceCollapsible>
  )
}

/** Several reads and searches in a row, summarised as one exploration task. */
function ExploreRow({ group }: { group: ExploreGroup }) {
  const { status } = useContext(TurnContext)
  const live = status === 'streaming'
  const running = group.tools.some((t) => toolLineStatus(t, live) === 'running')
  const failed = group.tools.some((t) => t.status === 'failed')
  const count = (kind: string) => group.tools.filter((t) => t.toolKind === kind).length
  const summary = [
    count('read') && `读取 ${count('read')} 个文件`,
    count('search') && `${count('search')} 次搜索`,
    count('fetch') && `获取 ${count('fetch')} 项资料`,
  ]
    .filter(Boolean)
    .join('，')
  return (
    <TraceCollapsible>
      <TraceCollapsibleTrigger>
        <TraceLine status={running ? 'running' : failed ? 'error' : undefined}>
          <TraceIcon>
            <TelescopeIcon />
          </TraceIcon>
          <TraceTitle>{running ? '正在探索' : '探索'}</TraceTitle>
          <TraceDescription>{summary}</TraceDescription>
        </TraceLine>
      </TraceCollapsibleTrigger>
      <TraceCollapsibleContent>
        <Trace>
          {group.tools.map((t) => (
            <LiveToolRow key={t.id} tool={t} />
          ))}
        </Trace>
      </TraceCollapsibleContent>
    </TraceCollapsible>
  )
}

/** A delegated agent the provider reported without a linking tool call. */
function SubagentRow({ group }: { group: SubagentGroup }) {
  const { status } = useContext(TurnContext)
  const running = status === 'streaming'
  return (
    <TraceCollapsible defaultOpen>
      <TraceCollapsibleTrigger>
        <TraceLine status={running ? 'running' : undefined}>
          <TraceIcon>
            <BotIcon />
          </TraceIcon>
          <TraceTitle>子代理{group.title ? `：${group.title}` : ''}</TraceTitle>
        </TraceLine>
      </TraceCollapsibleTrigger>
      <TraceCollapsibleContent keepMounted className="flex flex-col gap-3">
        <Rows rows={group.rows} />
      </TraceCollapsibleContent>
    </TraceCollapsible>
  )
}
