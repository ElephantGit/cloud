import { useState } from 'react'
import { Markdown } from '@/components/agent-thread/markdown'

/** Decode a report envelope without discarding other output fields. Never interpret HTML. */
export function ScheduleOutput({ text }: { text: string }) {
  const [rawOpen, setRawOpen] = useState(false)
  let value: unknown = text
  try { value = JSON.parse(text) } catch { /* Plain text is already readable. */ }
  const fields = value !== null && typeof value === 'object' && !Array.isArray(value)
    ? Object.entries(value) : null
  return <div className="sp-output">
    <div className="sp-output-content">
      {fields?.length ? fields.map(([name, content]) => <section key={name}>
        <h4>{name}</h4>
        {typeof content === 'string' ? <Markdown text={content}/> : <pre>{JSON.stringify(content, null, 2)}</pre>}
      </section>) : typeof value === 'string' ? <Markdown text={value}/> : <pre>{JSON.stringify(value, null, 2)}</pre>}
    </div>
    <details className="sp-output-source" onToggle={event => setRawOpen(event.currentTarget.open)}><summary>查看原始输出</summary>{rawOpen && <pre>{text}</pre>}</details>
  </div>
}
