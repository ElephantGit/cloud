import { BookmarkIcon, CpuIcon, RotateCcwIcon } from 'lucide-react'
import { Fragment } from 'react'
import { Button } from '@/components/ui/button'
import { HoverCard, HoverCardContent, HoverCardTrigger } from '@/components/ui/hover-card'
import { Marker, MarkerContent, MarkerIcon } from '@/components/ui/marker'
import {
  MessageScroller,
  MessageScrollerButton,
  MessageScrollerContent,
  MessageScrollerItem,
  MessageScrollerProvider,
  MessageScrollerViewport,
} from '@/components/ui/message-scroller'
import { Progress } from '@/components/ui/progress'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { isResponding } from '../model/conversation'
import type { Conversation, Marker as MarkerData, Turn } from '../model/types'
import { formatTokens } from './format'
import { subtleScrollbar } from '@/lib/scrollbar'
import { AgentTurn, UserPrompt } from './turn'

/** The conversation: each prompt anchors the scroll, so a new question settles near the top. */
export function Thread({ conversation, onAnswer }: { conversation: Conversation; onAnswer: (permissionId: string, optionId: string) => void }) {
  const markersAfter = (n: number) => conversation.markers.filter((m) => m.afterTurn === n)
  return (
    <MessageScrollerProvider autoScroll>
      <MessageScroller>
        <MessageScrollerViewport className={subtleScrollbar}>
          <MessageScrollerContent aria-busy={isResponding(conversation)} className="mx-auto w-full max-w-[760px] gap-6 px-3 pt-5 pb-4 sm:px-5 sm:pt-8">
            {markersAfter(0).map((m) => (
              <MarkerRow key={m.id} marker={m} />
            ))}
            {conversation.turns.map((turn, i) => (
              <Fragment key={turn.id}>
                <MessageScrollerItem messageId={`${turn.id}-prompt`} scrollAnchor>
                  <UserPrompt turn={turn} />
                </MessageScrollerItem>
                <MessageScrollerItem messageId={turn.id}>
                  <AgentTurn turn={turn} permissions={permissionsOf(conversation, turn)} onAnswer={onAnswer} />
                </MessageScrollerItem>
                {markersAfter(i + 1).map((m) => (
                  <MarkerRow key={m.id} marker={m} />
                ))}
              </Fragment>
            ))}
          </MessageScrollerContent>
        </MessageScrollerViewport>
        <MessageScrollerButton aria-label="滚动到底部" />
      </MessageScroller>
    </MessageScrollerProvider>
  )
}

function permissionsOf(conversation: Conversation, turn: Turn) {
  const tools = new Set(turn.items.flatMap((i) => (i.kind === 'tool' ? [i.id] : [])))
  return conversation.permissions.filter((p) => tools.has(p.toolCall.toolCallId))
}

/** Why the session above ended, said where the Agent's memory of it stops. */
const sessionEnd: Record<Extract<MarkerData, { kind: 'session' }>['reason'], string> = {
  stopped: '已停止',
  idle: '空闲太久，已释放工作目录',
  deleted: '会话已结束',
  interrupted: '服务重启，会话已中断',
}

function MarkerRow({ marker }: { marker: MarkerData }) {
  if (marker.kind === 'session') {
    return (
      <Marker variant="separator">
        <MarkerIcon>
          <RotateCcwIcon />
        </MarkerIcon>
        <MarkerContent>{sessionEnd[marker.reason]} · 之后的消息由 Agent 重新开始，不记得上面的内容</MarkerContent>
      </Marker>
    )
  }
  if (marker.kind === 'model') {
    return (
      <Marker variant="separator">
        <MarkerIcon>
          <CpuIcon />
        </MarkerIcon>
        <MarkerContent>已切换到 {marker.model}</MarkerContent>
      </Marker>
    )
  }
  return (
    <Marker variant="separator">
      <MarkerIcon>
        <BookmarkIcon />
      </MarkerIcon>
      <MarkerContent className="inline-flex items-center gap-2">
        检查点：{marker.label}
        <Tooltip>
          <TooltipTrigger render={<Button size="xs" variant="outline" aria-disabled className="cursor-not-allowed opacity-60" />}>恢复</TooltipTrigger>
          <TooltipContent>仅样例：恢复检查点要等后端支持</TooltipContent>
        </Tooltip>
      </MarkerContent>
    </Marker>
  )
}

/** The landing copy above a chat's input until its first prompt, as in desktop. */
export function LandingHeading() {
  return (
    <div className="mb-7">
      <h2 className="text-2xl font-medium tracking-[-0.035em] text-foreground sm:text-[28px]">今天想构建什么？</h2>
      <p className="mt-2 text-sm text-muted-foreground">描述一个任务，Ora 会读取代码、修改文件并运行命令。</p>
    </div>
  )
}

/** How full the context window is, from the agent's last report; details on hover. */
export function ContextUsage({ conversation }: { conversation: Conversation }) {
  const context = conversation.context
  if (!context) return null
  const percent = Math.min(100, Math.round((context.used / context.size) * 100))
  const last = conversation.turns.findLast((t) => t.usage)?.usage
  const rows = last
    ? [
        ['输入', last.inputTokens],
        ['输出', last.outputTokens],
        ['思考', last.thoughtTokens],
        ['缓存命中', last.cachedReadTokens],
      ].filter((r): r is [string, number] => typeof r[1] === 'number')
    : []
  return (
    <HoverCard>
      <HoverCardTrigger delay={10} closeDelay={100} render={<Button type="button" variant="ghost" size="sm" className="tabular-nums text-muted-foreground" />}>
        <ContextRing percent={percent} />
        {percent}%
      </HoverCardTrigger>
      <HoverCardContent side="bottom" align="end" className="flex w-64 flex-col gap-3 text-sm">
        <div className="flex items-center justify-between gap-3">
          <span className="font-medium">上下文</span>
          <span className="text-muted-foreground">
            {formatTokens(context.used)} / {formatTokens(context.size)}
          </span>
        </div>
        <Progress value={percent} aria-label="上下文用量" />
        {rows.length > 0 && (
          <div className="flex flex-col gap-1">
            <span className="text-xs text-muted-foreground">上一轮</span>
            {rows.map(([label, value]) => (
              <div key={label} className="flex justify-between gap-3">
                <span className="text-muted-foreground">{label}</span>
                <span className="tabular-nums">{formatTokens(value)}</span>
              </div>
            ))}
          </div>
        )}
      </HoverCardContent>
    </HoverCard>
  )
}

function ContextRing({ percent }: { percent: number }) {
  const r = 6
  const c = 2 * Math.PI * r
  return (
    <svg viewBox="0 0 16 16" className="size-4 -rotate-90" aria-hidden>
      <circle cx="8" cy="8" r={r} fill="none" strokeWidth="2" className="stroke-muted-foreground/25" />
      <circle
        cx="8"
        cy="8"
        r={r}
        fill="none"
        strokeWidth="2"
        strokeDasharray={c}
        strokeDashoffset={c * (1 - percent / 100)}
        className={percent >= 80 ? 'stroke-destructive' : 'stroke-foreground'}
      />
    </svg>
  )
}
