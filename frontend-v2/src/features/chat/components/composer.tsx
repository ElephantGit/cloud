import {
  ArrowUpIcon,
  CheckIcon,
  ChevronDownIcon,
  ListTodoIcon,
  PaperclipIcon,
  PlusIcon,
  PuzzleIcon,
  ShieldCheckIcon,
  ShieldHalfIcon,
  ShieldIcon,
  SquareIcon,
  XIcon,
} from 'lucide-react'
import { type ReactNode, useRef, useState } from 'react'
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Button } from '@/components/ui/button'
import { Trace, TraceIcon, TraceLine, TraceTitle } from '@/components/ui/trace'
import type { Attachment, PluginRef } from '../model/types'
import type { Agent, Draft } from '../store'
import { AttachmentList, PluginList } from './attachments'
import { ModelSelector } from './model-selector'

interface ComposerProps {
  draft: Draft
  onDraft: (draft: Draft) => void
  onSend: (draft: Draft) => void
  onStop: () => void
  responding: boolean
  queued: string[]
  onUnqueue: (index: number) => void
  agents: Agent[]
  agent: string
  model: string
  onChoose: (agent: string, model: string) => void
  plugins: PluginRef[]
  /** Only text can be sent: the `+` menu and `@` plugins are hidden, and pasted files ignored. */
  textOnly?: boolean
  /** The row above the box saying where a new chat will run; only before its first prompt. */
  context?: ReactNode
}

/**
 * The prompt input, laid out as desktop's: the text, then a footer with `+` (files and plugins)
 * and the permission mode on the left, the agent and model and send or stop on the right. Typing `@`
 * offers plugins by name. While the agent works a new prompt is queued, not sent.
 */
export function Composer(props: ComposerProps) {
  const { draft, onDraft, onSend, responding, queued, textOnly } = props
  const plugins = textOnly ? [] : props.plugins
  const fileInput = useRef<HTMLInputElement>(null)
  const form = useRef<HTMLFormElement>(null)
  const [highlight, setHighlight] = useState(0)
  const mention = draft.text.match(/(?:^|\s)@([^\s@]*)$/)
  const matches = mention
    ? plugins.filter((p) => !draft.plugins.some((x) => x.id === p.id) && p.name.toLowerCase().includes(mention[1].toLowerCase()))
    : []
  const canSend = draft.text.trim().length > 0

  const addPlugin = (p: PluginRef, fromMention = false) => {
    if (draft.plugins.some((x) => x.id === p.id)) return
    const text = fromMention && mention ? draft.text.slice(0, draft.text.length - mention[1].length - 1) : draft.text
    onDraft({ ...draft, text, plugins: [...draft.plugins, p] })
  }
  const togglePlugin = (p: PluginRef) =>
    draft.plugins.some((x) => x.id === p.id)
      ? onDraft({ ...draft, plugins: draft.plugins.filter((x) => x.id !== p.id) })
      : addPlugin(p)

  const addFiles = (files: FileList | null) => {
    if (!files?.length || textOnly) return
    const added: Attachment[] = [...files].map((f, i) => ({
      id: `${f.name}-${f.size}-${Date.now()}-${i}`,
      name: f.name,
      mimeType: f.type || 'application/octet-stream',
      size: f.size,
      url: f.type.startsWith('image/') ? URL.createObjectURL(f) : undefined,
    }))
    onDraft({ ...draft, attachments: [...draft.attachments, ...added] })
  }

  return (
    <div className="flex flex-col">
      {queued.length > 0 && <Queue items={queued} onRemove={props.onUnqueue} />}
      {props.context && (
        <div data-slot="composer-context" className="mb-1 flex h-6 items-center px-1">
          {props.context}
        </div>
      )}
      <form
        ref={form}
        className="relative flex flex-col rounded-xl border border-border bg-card shadow-[0_1px_3px_rgba(0,0,0,0.06),0_8px_24px_rgba(0,0,0,0.04)] transition-[border-color,box-shadow] duration-200 focus-within:border-foreground/30 focus-within:shadow-[0_2px_4px_rgba(0,0,0,0.07),0_12px_32px_rgba(0,0,0,0.07)] focus-within:ring-2 focus-within:ring-ring/25 hover:border-foreground/20 hover:shadow-[0_2px_4px_rgba(0,0,0,0.06),0_10px_28px_rgba(0,0,0,0.06)] dark:shadow-[0_1px_3px_rgba(0,0,0,0.28),0_10px_28px_rgba(0,0,0,0.18)]"
        onSubmit={(e) => {
          e.preventDefault()
          if (!canSend) return
          onSend(draft)
        }}
      >
        {matches.length > 0 && (
          <div role="listbox" aria-label="选择插件" className="absolute bottom-full left-0 z-20 mb-1 flex w-56 flex-col rounded-lg border bg-popover p-1 text-sm shadow-md">
            {matches.map((p, i) => (
              <button
                key={p.id}
                type="button"
                role="option"
                aria-selected={i === highlight % matches.length}
                className="flex items-center gap-2 rounded-md px-2 py-1.5 text-left aria-selected:bg-accent"
                onMouseDown={(e) => {
                  e.preventDefault()
                  addPlugin(p, true)
                }}
              >
                <PuzzleIcon className="size-4 text-muted-foreground" />
                {p.name}
              </button>
            ))}
          </div>
        )}
        <div className="flex flex-col p-2">
          {(draft.attachments.length > 0 || draft.plugins.length > 0) && (
            <div className="flex flex-col gap-2 px-2 pt-1 pb-2">
              <AttachmentList files={draft.attachments} onRemove={(id) => onDraft({ ...draft, attachments: draft.attachments.filter((a) => a.id !== id) })} />
              <PluginList items={draft.plugins} onRemove={(id) => onDraft({ ...draft, plugins: draft.plugins.filter((p) => p.id !== id) })} />
            </div>
          )}
          <textarea
            aria-label="消息"
            rows={2}
            placeholder={responding ? '继续输入，会在本轮结束后发送…' : textOnly ? '描述一个任务…' : '描述一个任务，输入 @ 选择插件…'}
            className="field-sizing-content max-h-[200px] min-h-14 w-full resize-none bg-transparent px-2 py-1 text-[15px] leading-normal outline-none placeholder:text-muted-foreground"
            value={draft.text}
            onChange={(e) => {
              setHighlight(0)
              onDraft({ ...draft, text: e.target.value })
            }}
            onPaste={(e) => addFiles(e.clipboardData.files)}
            onKeyDown={(e) => {
              // An IME composing Chinese also reports Enter; only a finished composition sends.
              if (e.nativeEvent.isComposing) return
              if (matches.length > 0) {
                if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
                  e.preventDefault()
                  setHighlight((h) => h + (e.key === 'ArrowDown' ? 1 : matches.length - 1))
                  return
                }
                if (e.key === 'Enter' || e.key === 'Tab') {
                  e.preventDefault()
                  addPlugin(matches[highlight % matches.length], true)
                  return
                }
              }
              if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault()
                form.current?.requestSubmit()
              }
            }}
          />
          <div className="flex min-h-8 items-center justify-between gap-2 pt-0.5">
            <div className="flex min-w-0 items-center gap-1">
              <input ref={fileInput} type="file" multiple hidden aria-label="选择文件" onChange={(e) => (addFiles(e.target.files), (e.target.value = ''))} />
              {!textOnly && (
              <DropdownMenu>
                <DropdownMenuTrigger
                  render={
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon-sm"
                      aria-label="添加"
                      className="rounded-full text-muted-foreground [&_svg]:transition-transform data-popup-open:[&_svg]:rotate-45"
                    />
                  }
                >
                  <PlusIcon />
                </DropdownMenuTrigger>
                <DropdownMenuContent align="start" side="top" className="w-48">
                  <DropdownMenuItem onClick={() => fileInput.current?.click()}>
                    <PaperclipIcon />
                    添加文件或图片
                  </DropdownMenuItem>
                  <DropdownMenuSeparator />
                  <DropdownMenuSub>
                    <DropdownMenuSubTrigger>
                      <PuzzleIcon />
                      插件
                    </DropdownMenuSubTrigger>
                    <DropdownMenuSubContent>
                      {plugins.map((p) => (
                        <DropdownMenuCheckboxItem key={p.id} checked={draft.plugins.some((x) => x.id === p.id)} onCheckedChange={() => togglePlugin(p)}>
                          {p.name}
                        </DropdownMenuCheckboxItem>
                      ))}
                    </DropdownMenuSubContent>
                  </DropdownMenuSub>
                </DropdownMenuContent>
              </DropdownMenu>
              )}
              <PermissionSelector />
            </div>
            <div className="flex shrink-0 items-center gap-2">
              <ModelSelector agents={props.agents} agent={props.agent} model={props.model} onChoose={props.onChoose} />
              {responding && !canSend ? (
                <Button type="button" size="icon" aria-label="停止" className={sendButton} onClick={props.onStop}>
                  <SquareIcon className="size-3.5 fill-current" />
                </Button>
              ) : (
                <Button type="submit" size="icon" aria-label={responding ? '加入队列' : '发送'} disabled={!canSend} className={sendButton}>
                  <ArrowUpIcon className="size-[18px]" />
                </Button>
              )}
            </div>
          </div>
        </div>
      </form>
    </div>
  )
}

const footerControl = 'h-7 gap-1.5 rounded-md px-2 text-xs font-normal text-muted-foreground hover:bg-muted/60 hover:text-foreground'
const sendButton =
  'size-8 rounded-full bg-foreground text-background shadow-sm transition-[background-color,color,box-shadow] duration-200 hover:bg-foreground/85 hover:shadow-md disabled:bg-muted disabled:text-muted-foreground disabled:opacity-100 disabled:shadow-none'

const policies = [
  { label: '每次询问', Icon: ShieldIcon },
  { label: '高风险确认', Icon: ShieldHalfIcon },
  { label: '完全访问', Icon: ShieldCheckIcon },
]

/**
 * How much the agent may do without asking. As in desktop only full access works for now; the
 * stricter modes are listed but disabled so the choice is visible.
 */
function PermissionSelector() {
  const current = policies[2]
  return (
    <DropdownMenu>
      <DropdownMenuTrigger render={<Button type="button" variant="ghost" size="sm" aria-label="权限模式" className={footerControl} />}>
        <current.Icon className="size-3.5 shrink-0" />
        <span className="whitespace-nowrap">{current.label}</span>
        <ChevronDownIcon className="size-3 shrink-0 opacity-50" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" side="top" className="w-44">
        {policies.map(({ label, Icon }) => (
          <DropdownMenuItem key={label} disabled={label !== current.label} className="gap-1.5 text-xs">
            <Icon className="size-3.5" />
            {label}
            {label === current.label && <CheckIcon className="ml-auto size-4" />}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

/** Prompts waiting for the agent to finish its turn, sent in order. */
function Queue({ items, onRemove }: { items: string[]; onRemove: (index: number) => void }) {
  return (
    <div className="mb-2 rounded-lg border bg-muted/40 px-3 py-2" aria-label="排队中的消息">
      <Trace>
        <TraceLine>
          <TraceIcon>
            <ListTodoIcon />
          </TraceIcon>
          <TraceTitle>{items.length} 条消息排队中，本轮结束后依次发送</TraceTitle>
        </TraceLine>
        {items.map((text, i) => (
          <div key={`${i}-${text}`} className="flex items-center gap-2 pl-6 text-sm">
            <span className="min-w-0 flex-1 truncate">{text}</span>
            <button type="button" className="text-muted-foreground hover:text-foreground" aria-label={`取消排队：${text}`} onClick={() => onRemove(i)}>
              <XIcon className="size-3.5" />
            </button>
          </div>
        ))}
      </Trace>
    </div>
  )
}
