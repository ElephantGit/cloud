import { useRef, useState } from 'react'
import { Check, ListTodo, Search } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldError, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import type { SourceTask } from './schedule-model'

/** Inline picker preserves the drawer's single scroll region and keyboard focus. */
export function SourceTaskPicker({
  tasks,
  value,
  onChange,
  error,
}: {
  tasks: SourceTask[]
  value: string
  onChange: (id: string) => void
  error?: string
}) {
  const [query, setQuery] = useState('')
  const [project, setProject] = useState('')
  const [picking, setPicking] = useState(false)
  const changeButton = useRef<HTMLButtonElement>(null)
  const searchInput = useRef<HTMLInputElement>(null)
  const selected = tasks.find((t) => t.id === value)
  const filtered = tasks.filter(
    (t) => (!project || t.projectId === project) && `${t.id} ${t.title}`.toLowerCase().includes(query.trim().toLowerCase()),
  )
  const choose = (id: string) => {
    onChange(id)
    setPicking(false)
    requestAnimationFrame(() => changeButton.current?.focus())
  }
  const change = () => {
    setPicking(true)
    setQuery('')
    setProject('')
    requestAnimationFrame(() => searchInput.current?.focus())
  }
  return (
    <Field data-invalid={!!error}>
      <div className="sp-source-label">
        <FieldLabel htmlFor="schedule-sourceTaskId">选择任务</FieldLabel>
        <Badge variant="outline">演示数据</Badge>
      </div>
      {selected && !picking ? (
        <div className="sp-source-card">
          <div className="sp-source-heading">
            <ListTodo size={17} />
            <strong>{selected.title}</strong>
            <Badge variant="outline">{selected.status}</Badge>
          </div>
          <p className="sp-source-meta">
            {selected.id} · {selected.projectName}
          </p>
          <p className="sp-source-description">
            {selected.description.slice(0, 120)}
            {selected.description.length > 120 ? '…' : ''}
          </p>
          {selected.description.length > 120 && (
            <details>
              <summary>展开完整描述</summary>
              <p className="sp-source-full">{selected.description}</p>
            </details>
          )}
          <div className="sp-source-actions">
            <Button
              ref={changeButton}
              aria-label="更换任务"
              id="schedule-sourceTaskId"
              type="button"
              size="sm"
              variant="outline"
              aria-invalid={!!error}
              aria-describedby={error ? 'error-sourceTaskId' : undefined}
              onClick={change}
            >
              更换任务
            </Button>
            <Button
              type="button"
              size="sm"
              variant="ghost"
              onClick={() => {
                onChange('')
                change()
              }}
            >
              清除选择
            </Button>
          </div>
        </div>
      ) : (
        <>
          <div className="sp-source-filters">
            <div className="sp-search">
              <Search size={15} aria-hidden="true" />
              <Input
                ref={searchInput}
                id="schedule-sourceTaskId"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder="搜索任务编号或名称"
                aria-invalid={!!error}
                aria-describedby={error ? 'error-sourceTaskId' : undefined}
              />
            </div>
            <NativeSelect aria-label="来源项目筛选" value={project} onChange={(e) => setProject(e.target.value)}>
              <NativeSelectOption value="">全部项目</NativeSelectOption>
              {[...new Map(tasks.map((t) => [t.projectId, t.projectName])).entries()].map(([id, name]) => (
                <NativeSelectOption key={id} value={id}>
                  {name}
                </NativeSelectOption>
              ))}
            </NativeSelect>
          </div>
          <ToggleGroup
            className="sp-workflow-list sp-source-list w-full"
            orientation="vertical"
            aria-label="可选 Cloud 任务"
            value={[value]}
            spacing={1}
            onValueChange={(values) => {
              if (values[0]) choose(values[0])
            }}
          >
            {filtered.map((t) => (
              <ToggleGroupItem
                key={t.id}
                value={t.id}
                disabled={!!t.unavailable}
                aria-label={`${t.title} ${t.id} ${t.status}`}
                className="sp-workflow-option h-auto w-full justify-start gap-3 p-3"
              >
                <ListTodo />
                <span>
                  <strong>{t.title}</strong>
                  <small>
                    {t.id} · {t.projectName}
                  </small>
                  {t.unavailable && <small>{t.unavailable}</small>}
                </span>
                <Badge variant="outline">{t.status}</Badge>
                {value === t.id && <Check />}
              </ToggleGroupItem>
            ))}
          </ToggleGroup>
          {!filtered.length && <p className="sp-empty-small">{tasks.length ? '没有匹配的任务，试试其他关键词或项目。' : '暂无可用任务'}</p>}
          {tasks.length > 0 && !tasks.some((t) => !t.unavailable) && <FieldDescription>暂无可用任务</FieldDescription>}
          {selected && (
            <Button
              type="button"
              variant="ghost"
              size="sm"
              onClick={() => {
                setPicking(false)
                requestAnimationFrame(() => changeButton.current?.focus())
              }}
            >
              保留当前任务
            </Button>
          )}
        </>
      )}
      <FieldError id="error-sourceTaskId">{error}</FieldError>
    </Field>
  )
}
