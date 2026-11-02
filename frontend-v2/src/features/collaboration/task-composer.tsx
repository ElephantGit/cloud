import { useMemo, useRef, useState } from 'react'
import { AtSign, Bot, SendHorizontal, X } from 'lucide-react'
import { useGetApiV1TenantsTidCollaborationTargets } from '@/api/tenants/tenants'
import type { CollaborationTarget } from '@/api/generated.schemas'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { agentReach } from './task-views'
import type { Task, TaskMentionRef } from './tasks-api/generated'

/** A comment as the composer hands it over. */
export interface CommentDraft {
  body: string
  mentions: TaskMentionRef[]
  toAgent: boolean
}

type Picked = TaskMentionRef & { displayName: string }

/**
 * The task's one input. Text goes to the team as a comment; picking the task's Agent from the @
 * menu, or turning on 发给 Agent, also sends it to the Agent as a turn. Only a target picked from
 * the menu counts: "@name" typed by hand routes nothing.
 */
export function TaskComposer({ tenantId, task, me, sending, error, onSend }: {
  tenantId: string
  task: Task
  me: string
  sending: boolean
  error: string
  onSend: (draft: CommentDraft) => Promise<boolean>
}) {
  const [body, setBody] = useState('')
  const [picked, setPicked] = useState<Picked[]>([])
  const [toAgent, setToAgent] = useState(false)
  const [query, setQuery] = useState<string | null>(null)
  const input = useRef<HTMLTextAreaElement>(null)
  const targets = useGetApiV1TenantsTidCollaborationTargets(tenantId, { query: { enabled: !!tenantId, staleTime: 60_000 } })
  const reach = agentReach(task, me)
  const sendToAgent = toAgent && reach.ok
  const options = useMemo(() => {
    if (query === null) return []
    const q = query.toLocaleLowerCase()
    return (targets.data?.items ?? []).filter(t => t.displayName.toLocaleLowerCase().includes(q) || t.description.toLocaleLowerCase().includes(q)).slice(0, 8)
  }, [query, targets.data])

  const typed = (value: string, caret: number) => {
    setBody(value)
    const before = value.slice(0, caret)
    const at = /(?:^|\s)@([^\s@]*)$/.exec(before)
    setQuery(at ? at[1] : null)
  }
  const choose = (target: CollaborationTarget) => {
    const el = input.current
    const caret = el?.selectionStart ?? body.length
    const before = body.slice(0, caret).replace(/@([^\s@]*)$/, `@${target.displayName} `)
    setBody(before + body.slice(caret))
    setQuery(null)
    if (target.type === 'agent') setToAgent(true)
    if (target.type === 'user' || target.type === 'agent') {
      const type = target.type
      setPicked(list => (list.some(p => p.type === type && p.id === target.id) ? list : [...list, { type, id: target.id, displayName: target.displayName }]))
    }
    el?.focus()
  }
  const agentEnabled = (target: CollaborationTarget) => target.type !== 'agent' || (task.assignee?.type === 'agent' && task.assignee.id === target.id && reach.ok)
  const submit = async () => {
    const text = body.trim()
    if (!text || sending) return
    const ok = await onSend({ body: text, mentions: picked.map(({ type, id }) => ({ type, id })), toAgent: sendToAgent })
    if (ok) {
      setBody('')
      setPicked([])
      setToAgent(false)
    }
  }

  return <div className="mc-composer">
    {picked.length > 0 && <div className="mc-chips" aria-label="已 @ 的对象">
      {picked.map(p => <span key={`${p.type}:${p.id}`} className={`mc-chip ${p.type}`}>
        {p.type === 'agent' ? <Bot size={12} /> : <AtSign size={12} />}{p.displayName}
        <button type="button" aria-label={`取消 @${p.displayName}`} onClick={() => { setPicked(list => list.filter(x => x !== p)); if (p.type === 'agent') setToAgent(false) }}><X size={11} /></button>
      </span>)}
    </div>}
    <div className="mc-composer-box">
      <Textarea ref={input} aria-label="评论或给 Agent 的消息" placeholder={sendToAgent ? '告诉 Agent 下一步做什么…' : '写评论，输入 @ 提到成员或 Agent…'} value={body} rows={3}
        onChange={e => typed(e.target.value, e.target.selectionStart)}
        onKeyDown={e => {
          if (e.key === 'Escape') setQuery(null)
          if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) { e.preventDefault(); void submit() }
        }} />
      {query !== null && options.length > 0 && <ul className="mc-mentions" role="listbox" aria-label="@ 对象">
        {options.map(t => <li key={`${t.type}:${t.id}`}>
          <button type="button" role="option" aria-selected="false" disabled={!agentEnabled(t)} onMouseDown={e => e.preventDefault()} onClick={() => choose(t)}>
            {t.type === 'agent' ? <Bot size={14} /> : <AtSign size={14} />}
            <span><b>{t.displayName}</b><small>{t.type === 'agent' ? (agentEnabled(t) ? '发给 Agent' : task.assignee?.id === t.id ? reach.reason : '没有指派给这个任务') : '提及'}</small></span>
          </button>
        </li>)}
      </ul>}
    </div>
    <div className="mc-composer-foot">
      {task.assignee?.type === 'agent'
        ? <label className="mc-switch-line" title={reach.reason}><Switch checked={sendToAgent} disabled={!reach.ok} onCheckedChange={setToAgent} aria-label="发给 Agent" />发给 Agent<small>{reach.reason}</small></label>
        : <small className="mc-muted">评论全团队可见。</small>}
      <Button type="button" size="sm" disabled={!body.trim() || sending} onClick={() => void submit()}>
        <SendHorizontal data-icon="inline-start" />{sending ? '发送中…' : sendToAgent ? '发给 Agent' : '评论'}
      </Button>
    </div>
    {error && <p role="alert" className="mc-error">{error}</p>}
  </div>
}
