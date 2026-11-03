import { AtSign, Bot, MessageSquare, Play } from 'lucide-react'
import { formatTime } from './squad-model'
import { runEndLabel } from './task-model'
import type { TaskComment, TaskRun } from './tasks-api/generated'

type Entry = { at: string; comment: TaskComment } | { at: string; run: TaskRun; index: number }

/**
 * The team's side of the task in time order: comments, and when each run started. A comment sent
 * to the Agent also appears in that run's thread as its prompt; here it says so.
 */
export function TaskDiscussion({ comments, runs, onRun }: { comments: TaskComment[]; runs: TaskRun[]; onRun: (id: string) => void }) {
  const entries: Entry[] = [
    ...comments.map(comment => ({ at: comment.createdAt, comment })),
    ...runs.map((run, index) => ({ at: run.createdAt, run, index })),
  ].sort((a, b) => a.at.localeCompare(b.at))
  if (entries.length === 0) {
    return <div className="mc-run-empty"><MessageSquare size={22} aria-hidden="true" /><p>还没有讨论。评论全团队可见，@ 一个成员提醒他来看。</p></div>
  }
  const runIndex = new Map(runs.map((run, i) => [run.id, i + 1]))
  return <ol className="mc-discussion" aria-label="讨论">
    {entries.map(entry => 'comment' in entry
      ? <li key={entry.comment.id} className="mc-comment">
        <header><b>{entry.comment.authorName}</b><time>{formatTime(entry.comment.createdAt)}</time>
          {entry.comment.runId && <button type="button" className="mc-tag agent" onClick={() => onRun(entry.comment.runId)}><Bot size={11} />发给 Agent · 运行 {runIndex.get(entry.comment.runId)}</button>}
        </header>
        <p>{entry.comment.body}</p>
        {entry.comment.mentions.length > 0 && <div className="mc-chips">{entry.comment.mentions.map(m => <span key={`${m.type}:${m.id}`} className={`mc-chip ${m.type}`}>{m.type === 'agent' ? <Bot size={11} /> : <AtSign size={11} />}{m.displayName}</span>)}</div>}
      </li>
      : <li key={entry.run.id} className="mc-event">
        <Play size={12} aria-hidden="true" />
        <span><b>{entry.run.startedByName}</b> 用 {entry.run.executorName} 发起了运行 {entry.index + 1}{entry.run.state === 'ended' && `，${runEndLabel(entry.run.endReason)}`}</span>
        <button type="button" className="mc-link" onClick={() => onRun(entry.run.id)}>查看过程</button>
        <time>{formatTime(entry.run.createdAt)}</time>
      </li>)}
  </ol>
}
