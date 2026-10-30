import { useRef, useState } from 'react'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Field, FieldLabel } from '@/components/ui/field'
import { ScheduleDrawer } from './schedule-drawer'
import { ScheduleForm } from './schedule-form'
import { formatTime, ruleText, type Draft, type Task } from './schedule-model'
import { previewCreatedAt, useTaskPreview, type PreviewSchedule } from './task-preview-model'
import './schedules.css'

/** Explicit, isolated preview; never mounts the persistent configuration page. */
export default function TaskPreview() {
  const { sources, schedules, save, toggle, run } = useTaskPreview()
  const [panel, setPanel] = useState<'create' | 'edit' | 'detail'>()
  const [id, setId] = useState('')
  const [sourceId, setSourceId] = useState('')
  const [instruction, setInstruction] = useState('')
  const [query, setQuery] = useState('')
  const [project, setProject] = useState('')
  const [search, setSearch] = useState('')
  const [filter, setFilter] = useState('all')
  const [tab, setTab] = useState<'config' | 'history'>('config')
  const [selectedRun, setSelectedRun] = useState('')
  const [changed, setChanged] = useState(false)
  const closeForm = useRef<() => void>(() => setPanel(undefined))
  const task = schedules.find((x) => x.id === id)
  const source = sources.find((x) => x.id === sourceId)
  const currentSource = sources.find((x) => x.id === task?.target.sourceTaskId)
  const close = () => setPanel(undefined)
  const open = (mode: 'create' | 'edit', item?: PreviewSchedule) => {
    setId(item?.id ?? '')
    setSourceId(item?.target.sourceTaskId ?? '')
    setInstruction(item?.target.supplementalInstruction ?? '')
    setQuery('')
    setProject('')
    setChanged(false)
    setPanel(mode)
  }
  const invalid = !source ? '请选择一个可用任务' : source.unavailable
  const candidates = sources.filter(
    (x) => (!project || x.project === project) && `${x.id} ${x.title}`.toLowerCase().includes(query.trim().toLowerCase()),
  )
  const existing: Task | undefined =
    panel === 'edit' && task
      ? { ...task, projectId: '', target: { kind: 'chat', prompt: '使用来源任务内容', agent: '', model: '' }, runs: [] }
      : undefined
  const saveDraft = (draft: Draft) => {
    if (invalid || !source) return
    const next: PreviewSchedule = {
      id: task?.id ?? crypto.randomUUID(),
      name: draft.name.trim(),
      target: { kind: 'task', sourceTaskId: source.id, supplementalInstruction: instruction.trim() },
      rule: structuredClone(draft.rule),
      enabled: task?.enabled ?? true,
      createdAt: task?.createdAt ?? previewCreatedAt(),
      runs: task?.runs ?? [],
    }
    save(next)
    setId(next.id)
    setPanel('detail')
    setTab('config')
    setChanged(false)
  }
  const history = task?.runs.find((x) => x.id === selectedRun) ?? task?.runs[0]
  return (
    <div className="schedules-feature sp-page">
      <div className="sp-workspace-bar">
        <Badge variant="outline">前端演示 · 刷新恢复样例</Badge>
        <a className="text-sm underline" href="/schedules">
          返回配置管理
        </a>
      </div>
      <div className="sp-content">
        <header className="sp-page-header">
          <div>
            <h1>定时任务</h1>
            <p>让已有任务按计划独立执行，记录集中在这里。</p>
          </div>
          <Button onClick={() => open('create')}>新建定时任务</Button>
        </header>
        <div className="sp-filters">
          <Input aria-label="搜索定时任务" placeholder="搜索定时任务…" value={search} onChange={(e) => setSearch(e.target.value)} />
          <NativeSelect aria-label="执行类型筛选" value={filter} onChange={(e) => setFilter(e.target.value)}>
            <NativeSelectOption value="all">全部类型</NativeSelectOption>
            <NativeSelectOption value="task">已有任务</NativeSelectOption>
            <NativeSelectOption value="chat">直接对话</NativeSelectOption>
          </NativeSelect>
        </div>
        <div className="tp-list">
          {schedules
            .filter((x) => x.name.includes(search) && (filter === 'all' || filter === 'task'))
            .map((x) => (
              <button
                className="tp-row"
                key={x.id}
                onClick={() => {
                  setId(x.id)
                  setPanel('detail')
                  setTab('config')
                  setSelectedRun('')
                }}
              >
                <span>
                  <strong>{x.name}</strong>
                  <small>{sources.find((s) => s.id === x.target.sourceTaskId)?.title ?? '来源任务不可用'}</small>
                </span>
                <Badge variant="outline">已有任务</Badge>
                <span>{ruleText(x.rule)}</span>
                <span>{x.enabled ? '已启用（演示）' : '已暂停'}</span>
              </button>
            ))}
        </div>
        {!schedules.some((x) => x.name.includes(search) && (filter === 'all' || filter === 'task')) && (
          <p className="sp-empty-small">没有匹配的定时任务</p>
        )}
        <p className="sp-helper mt-4">此页面使用模拟数据，不会执行真实任务或增加普通对话记录。</p>
      </div>
      {panel && (
        <ScheduleDrawer
          title={panel === 'create' ? '新建定时任务' : panel === 'edit' ? '编辑定时任务' : (task?.name ?? '任务详情')}
          onClose={() => (panel === 'detail' ? setPanel(undefined) : closeForm.current())}
        >
          {panel !== 'detail' ? (
            <ScheduleForm
              key={`${panel}-${id}`}
              task={existing}
              data={{ projects: [], workflows: [] }}
              externalDirty={changed}
              sourceName={source?.title}
              sourceProject={source?.project}
              blocked={!!invalid}
              saveError={invalid}
              onSave={saveDraft}
              onCancel={close}
              onCloseReady={(fn) => {
                closeForm.current = fn
              }}
              taskContent={
                <>
                  <div className="tp-types">
                    <Button type="button" variant="outline" disabled>
                      直接对话 · 请返回配置管理
                    </Button>
                    <Badge>已有任务</Badge>
                    <Button type="button" variant="outline" disabled>
                      运行工作流 · 尚未接入
                    </Button>
                  </div>
                  <Field>
                    <FieldLabel htmlFor="task-search">选择任务</FieldLabel>
                    <Input id="task-search" placeholder="搜索编号或名称" value={query} onChange={(e) => setQuery(e.target.value)} />
                  </Field>
                  <NativeSelect aria-label="筛选来源项目" value={project} onChange={(e) => setProject(e.target.value)}>
                    <NativeSelectOption value="">全部项目</NativeSelectOption>
                    {[...new Set(sources.map((x) => x.project))].map((x) => (
                      <NativeSelectOption key={x} value={x}>
                        {x}
                      </NativeSelectOption>
                    ))}
                  </NativeSelect>
                  <div className="tp-picker" role="group" aria-label="来源任务">
                    {candidates.map((x) => (
                      <button
                        type="button"
                        key={x.id}
                        className="tp-option"
                        aria-label={`${x.title} ${x.id} ${x.status}`}
                        aria-pressed={sourceId === x.id}
                        disabled={!!x.unavailable}
                        onClick={() => {
                          setSourceId(x.id)
                          setChanged(true)
                        }}
                      >
                        <span>
                          <strong>{x.title}</strong>
                          <small>
                            {x.id} · {x.project}
                          </small>
                        </span>
                        <Badge variant="outline">{x.status}</Badge>
                        {sourceId === x.id && <span>✓</span>}
                        {x.unavailable && <small>{x.unavailable}</small>}
                      </button>
                    ))}
                  </div>
                  {!candidates.length && <p className="sp-empty-small">没有匹配的任务</p>}
                  {!sources.some((x) => !x.unavailable) && <p role="status">暂无可用任务</p>}
                  {source && (
                    <div className="sp-prompt-preview">
                      <strong>{source.title}</strong>
                      <details>
                        <summary>查看完整任务描述</summary>
                        <p>{source.description}</p>
                      </details>
                      <p>{source.description.slice(0, 70)}</p>
                      <Button
                        type="button"
                        variant="ghost"
                        onClick={() => {
                          setSourceId('')
                          setChanged(true)
                        }}
                      >
                        清除选择
                      </Button>
                    </div>
                  )}
                  <Field>
                    <FieldLabel htmlFor="task-instruction">补充说明（可选）</FieldLabel>
                    <Textarea
                      id="task-instruction"
                      value={instruction}
                      onChange={(e) => {
                        setInstruction(e.target.value)
                        setChanged(true)
                      }}
                      placeholder="例如：优先检查发布阻塞问题，输出简洁清单。"
                    />
                  </Field>
                  <p className="sp-helper">
                    每次使用任务最新内容独立执行，结果仅保存在定时任务历史。不会读取原任务的历史对话；已完成任务仍可周期执行。
                  </p>
                </>
              }
            />
          ) : (
            task && (
              <div className="sp-detail">
                <div className="sp-detail-toolbar">
                  <Badge variant="outline">{task.enabled ? '已启用（演示）' : '已暂停'}</Badge>
                  <Button variant="outline" onClick={() => open('edit', task)}>
                    编辑
                  </Button>
                  <Button variant="outline" onClick={() => toggle(task.id)}>
                    {task.enabled ? '暂停' : '恢复'}
                  </Button>
                </div>
                <div className="sp-tabs">
                  <button onClick={() => setTab('config')} aria-pressed={tab === 'config'}>
                    配置
                  </button>
                  <button onClick={() => setTab('history')} aria-pressed={tab === 'history'}>
                    运行历史（{task.runs.length}）
                  </button>
                </div>
                <div className="sp-detail-scroll">
                  {tab === 'config' ? (
                    <>
                      <Badge>已有任务</Badge>
                      <h3>{currentSource?.title ?? '来源任务不可用'}</h3>
                      <p className="sp-prompt-preview">{currentSource?.description}</p>
                      <p>关联项目：{currentSource?.project ?? '不可用'}</p>
                      <p>补充说明：{task.target.supplementalInstruction || '无'}</p>
                      <p>
                        {ruleText(task.rule)} · {task.rule.timezone}
                      </p>
                      <p className="sp-helper">每次读取最新描述和项目，历史保留当次内容。原任务完成不会暂停本安排。</p>
                    </>
                  ) : (
                    <>
                      {!history && <p>尚无运行记录，点击模拟运行体验。</p>}
                      <div className="sp-history-list">
                        {task.runs.map((r) => (
                          <button key={r.id} className={history?.id === r.id ? 'active' : ''} onClick={() => setSelectedRun(r.id)}>
                            {formatTime(r.at, task.rule.timezone)} ·{' '}
                            {r.status === 'running' ? '进行中' : r.status === 'success' ? '成功' : '失败'}（演示）
                          </button>
                        ))}
                      </div>
                      {history && (
                        <>
                          <h3>当次输入</h3>
                          <p>
                            {history.source.title} · {history.source.project}
                          </p>
                          <p className="sp-prompt-preview">{history.source.description}</p>
                          <p>补充说明：{history.instruction || '无'}</p>
                          <h3>运行结果（演示）</h3>
                          <p className="sp-prompt-preview" role="status">
                            {history.status === 'running' ? '正在模拟运行…' : history.output}
                          </p>
                        </>
                      )}
                    </>
                  )}
                  {(!currentSource || currentSource.unavailable) && (
                    <p role="alert">{currentSource?.unavailable ?? '来源任务不可用'}，请重新选择。既有历史仍可查看。</p>
                  )}
                </div>
                <footer className="sp-detail-footer">
                  <Button
                    disabled={!currentSource || !!currentSource.unavailable || task.runs.some((r) => r.status === 'running')}
                    onClick={() => {
                      run(task.id)
                      setSelectedRun('')
                      setTab('history')
                    }}
                  >
                    模拟运行
                  </Button>
                  <Button
                    variant="outline"
                    disabled={!currentSource || !!currentSource.unavailable || task.runs.some((r) => r.status === 'running')}
                    onClick={() => {
                      run(task.id, true)
                      setSelectedRun('')
                      setTab('history')
                    }}
                  >
                    模拟失败
                  </Button>
                </footer>
              </div>
            )
          )}
        </ScheduleDrawer>
      )}
    </div>
  )
}
