import type * as acp from '@agentclientprotocol/sdk'
import {
  BotIcon,
  BrainIcon,
  CheckIcon,
  CircleDashedIcon,
  FileCodeIcon,
  FilePenIcon,
  FileTextIcon,
  FlaskConicalIcon,
  GlobeIcon,
  LayersIcon,
  type LucideIcon,
  MoveRightIcon,
  SearchIcon,
  ShieldQuestionIcon,
  TerminalIcon,
  Trash2Icon,
  WrenchIcon,
  XIcon,
} from 'lucide-react'
import { type ReactNode, useState } from 'react'
import { Button } from '@/components/ui/button'
import { Item, ItemActions, ItemContent, ItemDescription, ItemMedia, ItemTitle } from '@/components/ui/item'
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
import type { PermissionRequest, ToolItem } from '../model/types'
import { DiffBlock, OutputBlock } from './code'
import { formatDuration } from './format'
import { isStackTrace, toolLineStatus } from './tool-status'

const icons: Record<acp.ToolKind, LucideIcon> = {
  read: FileTextIcon,
  edit: FilePenIcon,
  delete: Trash2Icon,
  move: MoveRightIcon,
  search: SearchIcon,
  execute: TerminalIcon,
  think: BrainIcon,
  fetch: GlobeIcon,
  switch_mode: LayersIcon,
  other: WrenchIcon,
}

interface ToolRowProps {
  tool: ToolItem
  live: boolean
  permission?: PermissionRequest
  onAnswer?: (permissionId: string, optionId: string) => void
  /** A delegated child's rows, drawn inside the tool that started it. */
  subagent?: ReactNode
}

/** One tool call as a trace line; its input, output and diffs open beneath it. */
export function ToolRow({ tool, live, permission, onAnswer, subagent }: ToolRowProps) {
  const Icon = subagent ? BotIcon : icons[tool.toolKind ?? 'other']
  const details = subagent ?? <ToolDetails tool={tool} />
  const status = toolLineStatus(tool, live)
  const asking = permission && !permission.outcome
  // Failures open on their own, but only until the person opens or closes the row themselves.
  const [open, setOpen] = useState<boolean>()
  const line = (
    <TraceLine status={status}>
      <TraceIcon>{asking ? <ShieldQuestionIcon /> : <Icon />}</TraceIcon>
      <TraceTitle>{tool.title}</TraceTitle>
      <TraceDescription>{toolDescription(tool)}</TraceDescription>
    </TraceLine>
  )
  return (
    <>
      {details ? (
        <TraceCollapsible open={open ?? (!!subagent || tool.status === 'failed')} onOpenChange={setOpen}>
          <TraceCollapsibleTrigger>{line}</TraceCollapsibleTrigger>
          <TraceCollapsibleContent keepMounted={!!subagent} className="flex flex-col gap-2">
            {details}
          </TraceCollapsibleContent>
        </TraceCollapsible>
      ) : (
        line
      )}
      {permission && <PermissionCard request={permission} tool={tool} onAnswer={onAnswer} />}
    </>
  )
}

function toolDescription(tool: ToolItem): string {
  if (tool.status === 'cancelled') return '已取消'
  if (tool.status === 'failed') return '失败'
  if (tool.endedAt !== undefined) return formatDuration(tool.endedAt - tool.startedAt)
  return ''
}

/** What opens under a tool line; null when the call carries nothing worth showing. */
function ToolDetails({ tool }: { tool: ToolItem }) {
  const output = (tool.rawOutput ?? {}) as Record<string, unknown>
  const texts = tool.content.flatMap((c) => (c.type === 'content' && c.content.type === 'text' ? [c.content.text] : []))
  const diffs = tool.content.flatMap((c) => (c.type === 'diff' ? [c] : []))

  if (Array.isArray(output.suites)) return <TestResults suites={output.suites as Suite[]} />
  if (tool.status === 'failed' && texts.some(isStackTrace)) return <StackTrace stack={texts.find(isStackTrace)!} />

  const parts: ReactNode[] = []
  if (tool.toolKind === 'execute') {
    const stdout = [output.stdout, output.stderr].filter((s): s is string => typeof s === 'string' && s !== '').join('\n')
    const command = (tool.rawInput as { command?: string } | undefined)?.command
    const shown = stdout || texts.join('\n')
    if (command || shown) {
      parts.push(
        <OutputBlock
          key="terminal"
          error={tool.status === 'failed'}
          text={[command && `$ ${command}`, shown, typeof output.exitCode === 'number' && output.exitCode !== 0 ? `退出码 ${output.exitCode}` : '']
            .filter(Boolean)
            .join('\n')}
        />,
      )
    }
  } else if (texts.length) {
    parts.push(<OutputBlock key="text" text={texts.join('\n')} error={tool.status === 'failed'} />)
  }
  for (const d of diffs) parts.push(<DiffBlock key={`diff-${d.path}`} path={d.path} oldText={d.oldText} newText={d.newText} />)
  if (!parts.length && tool.locations.length) {
    parts.push(
      <Trace key="locations">
        {tool.locations.map((l) => (
          <TraceLine key={`${l.path}:${l.line ?? ''}`}>
            <TraceIcon>
              <FileCodeIcon />
            </TraceIcon>
            <TraceTitle className="font-mono text-xs">
              {l.path}
              {l.line ? `:${l.line}` : ''}
            </TraceTitle>
          </TraceLine>
        ))}
      </Trace>,
    )
  }
  return parts.length ? <>{parts}</> : null
}

/** Asks the person to allow a tool call; after the answer it stays as a one-line record. */
function PermissionCard({
  request,
  tool,
  onAnswer,
}: {
  request: PermissionRequest
  tool: ToolItem
  onAnswer?: (permissionId: string, optionId: string) => void
}) {
  if (request.outcome) {
    const chosen = 'optionId' in request.outcome ? request.options.find((o) => o.optionId === (request.outcome as { optionId: string }).optionId) : undefined
    return (
      <TraceLine status={chosen?.kind.startsWith('reject') ? 'error' : undefined}>
        <TraceIcon>{chosen && !chosen.kind.startsWith('reject') ? <CheckIcon /> : <XIcon />}</TraceIcon>
        <TraceTitle>{chosen ? `你选择了：${chosen.name}` : '未回答，轮次已结束'}</TraceTitle>
      </TraceLine>
    )
  }
  const command = (request.toolCall.rawInput as { command?: string } | undefined)?.command
  return (
    <Item variant="outline" size="sm" role="group" aria-label="权限确认">
      <ItemMedia variant="icon">
        <ShieldQuestionIcon />
      </ItemMedia>
      <ItemContent>
        <ItemTitle>允许 Agent {request.toolCall.title ?? tool.title}？</ItemTitle>
        <ItemDescription>{command ? <code className="font-mono">{command}</code> : '这个操作需要你的确认。'}</ItemDescription>
      </ItemContent>
      <ItemActions>
        {request.options.map((o) => (
          <Button
            key={o.optionId}
            size="sm"
            variant={o.kind === 'allow_once' ? 'default' : 'outline'}
            onClick={() => onAnswer?.(request.id, o.optionId)}
          >
            {o.name}
          </Button>
        ))}
      </ItemActions>
    </Item>
  )
}

interface Suite {
  name: string
  tests: { name: string; status: 'passed' | 'failed' | 'skipped'; durationMs?: number; error?: string }[]
}

/** Test suites a test run reported in its raw output: failures open, the rest one line each. */
function TestResults({ suites }: { suites: Suite[] }) {
  return (
    <Trace>
      {suites.map((suite) => {
        const failed = suite.tests.filter((t) => t.status === 'failed').length
        const skipped = suite.tests.filter((t) => t.status === 'skipped').length
        return (
          <TraceCollapsible key={suite.name} defaultOpen={failed > 0}>
            <TraceCollapsibleTrigger>
              <TraceLine status={failed ? 'error' : undefined}>
                <TraceIcon>
                  <FlaskConicalIcon />
                </TraceIcon>
                <TraceTitle className="font-mono text-xs">{suite.name}</TraceTitle>
                <TraceDescription>
                  {[`${suite.tests.length} 个测试`, failed && `${failed} 个失败`, skipped && `${skipped} 个跳过`].filter(Boolean).join('，')}
                </TraceDescription>
              </TraceLine>
            </TraceCollapsibleTrigger>
            <TraceCollapsibleContent>
              <Trace>
                {suite.tests.map((t) =>
                  t.status === 'failed' ? (
                    <TraceCollapsible key={t.name} defaultOpen>
                      <TraceCollapsibleTrigger>
                        <TraceLine status="error">
                          <TraceIcon>
                            <XIcon />
                          </TraceIcon>
                          <TraceTitle>{t.name}</TraceTitle>
                        </TraceLine>
                      </TraceCollapsibleTrigger>
                      <TraceCollapsibleContent>{t.error && <OutputBlock text={t.error} error />}</TraceCollapsibleContent>
                    </TraceCollapsible>
                  ) : (
                    <TraceLine key={t.name} status={t.status === 'skipped' ? 'pending' : undefined}>
                      <TraceIcon>{t.status === 'skipped' ? <CircleDashedIcon /> : <CheckIcon />}</TraceIcon>
                      <TraceTitle>{t.status === 'skipped' ? `跳过：${t.name}` : t.name}</TraceTitle>
                      {t.durationMs !== undefined && <TraceDescription>{t.durationMs} ms</TraceDescription>}
                    </TraceLine>
                  ),
                )}
              </Trace>
            </TraceCollapsibleContent>
          </TraceCollapsible>
        )
      })}
    </Trace>
  )
}

/** A stack trace with the app's own frames listed and runtime frames folded away. */
function StackTrace({ stack }: { stack: string }) {
  const [error, ...lines] = stack.split('\n').map((l) => l.trim())
  const frames = lines.filter((l) => l.startsWith('at ')).map((line) => {
    const m = line.match(/^at (.+?) \((.+):(\d+):(\d+)\)$/)
    return { line, name: m?.[1], file: m?.[2], lineNumber: m?.[3], internal: /node_modules|node:/.test(line) }
  })
  const own = frames.filter((f) => !f.internal)
  const internal = frames.filter((f) => f.internal)
  return (
    <div className="flex flex-col gap-2">
      <p className="font-mono text-xs text-destructive">{error}</p>
      <Trace>
        {own.map((f) => (
          <TraceLine key={f.line}>
            <TraceIcon>
              <FileCodeIcon />
            </TraceIcon>
            <TraceTitle className="font-mono text-xs">{f.name ? `${f.name}（${f.file}:${f.lineNumber}）` : f.line}</TraceTitle>
          </TraceLine>
        ))}
        {internal.length > 0 && (
          <TraceCollapsible>
            <TraceCollapsibleTrigger>
              <TraceLine>
                <TraceIcon>
                  <LayersIcon />
                </TraceIcon>
                <TraceTitle>{internal.length} 个运行时栈帧</TraceTitle>
              </TraceLine>
            </TraceCollapsibleTrigger>
            <TraceCollapsibleContent>
              <ol className="flex flex-col gap-1 overflow-x-auto rounded-md bg-muted p-3 font-mono text-xs">
                {internal.map((f) => (
                  <li key={f.line} className="whitespace-nowrap">
                    {f.line}
                  </li>
                ))}
              </ol>
            </TraceCollapsibleContent>
          </TraceCollapsible>
        )}
      </Trace>
    </div>
  )
}
