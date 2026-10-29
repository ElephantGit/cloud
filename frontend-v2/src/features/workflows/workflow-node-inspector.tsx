import { createContext, useContext, useId, useRef, useState } from 'react'
import type { CanvasNode } from './workflow-canvas'
import { formatTypedValue, parseTypedValue } from './workflow-typed-value'

const inputClass = 'w-full rounded-md border bg-background px-2 py-1.5 text-sm'
type Obj = Record<string, unknown>
type Edge = { source: string; target: string }
type Props = { node: CanvasNode; nodes?: CanvasNode[]; edges?: Edge[]; globalVariables?: unknown; onChange: (node: CanvasNode) => void; onClose: () => void; readOnly?: boolean }
const obj = (value: unknown): Obj => value && typeof value === 'object' && !Array.isArray(value) ? value as Obj : {}
const list = (value: unknown): Obj[] => Array.isArray(value) ? value.map(obj) : []
const str = (value: unknown) => typeof value === 'string' ? value : ''
const selectorText = (value: unknown) => Array.isArray(value) ? value.join('.') : ''
const selector = (value: string) => value === '' ? [] : value.split('.')
const types = ['string', 'number', 'integer', 'boolean', 'secret', 'file', 'object', 'any', 'array', 'array[string]', 'array[number]', 'array[object]', 'array[boolean]', 'array[file]', 'array[any]']

function Field({ label, children }: { label: string; children: React.ReactNode }) { return <label className="block space-y-1 text-xs font-medium"><span>{label}</span>{children}</label> }
function Text({ label, value, onChange, multiline = false }: { label: string; value: unknown; onChange: (value: string) => void; multiline?: boolean }) {
  return <Field label={label}>{multiline ? <textarea className={inputClass} rows={3} value={str(value)} onChange={(e) => onChange(e.target.value)} /> : <input className={inputClass} value={str(value)} onChange={(e) => onChange(e.target.value)} />}</Field>
}
function TypedValue({ value, valueType, onChange, label = '初始值（可选）' }: { value: unknown; valueType: unknown; onChange: (value: unknown) => void; label?: string }) {
  const type = str(valueType) || 'string'
  const formatted = formatTypedValue(value, type)
  const [text, setText] = useState(formatted)
  const [error, setError] = useState(false)
  return <Field label={label}><input type={type === 'secret' ? 'password' : 'text'} className={inputClass} value={text} onChange={(e) => { setText(e.target.value); setError(false) }} onBlur={() => { if (text === formatted) return; if (type === 'string' || type === 'secret') { onChange(text); setError(false); return } const parsed = parseTypedValue(text, type); if (parsed.valid) { onChange(parsed.value); setError(false) } else setError(true) }} placeholder={type === 'boolean' ? 'true / false' : '输入值'}/>{error && <span role="alert" className="text-red-600">请输入有效的 {type} 值。</span>}</Field>
}
function SchemaEditor({ value, onChange }: { value: unknown; onChange: (value: Obj) => void }) {
  const formatted = JSON.stringify(value ?? {}, null, 2)
  const [text, setText] = useState(formatted)
  const [error, setError] = useState(false)
  return <Field label="输出 JSON Schema"><textarea className={`${inputClass} font-mono`} rows={5} value={text} onChange={(e) => { setText(e.target.value); setError(false) }} onBlur={() => { try { const parsed: unknown = JSON.parse(text); if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) throw new Error(); onChange(parsed as Obj) } catch { setError(true) } }}/>{error && <span role="alert" className="text-red-600">请输入 JSON 对象。</span>}</Field>
}
function Choice({ label, value, options, onChange }: { label: string; value: unknown; options: [string, string][]; onChange: (value: string) => void }) {
  return <Field label={label}><select className={inputClass} value={str(value)} onChange={(e) => onChange(e.target.value)}>{options.map(([key, title]) => <option key={key} value={key}>{title}</option>)}</select></Field>
}
function NumberField({ label, value, onChange, min = 1, max }: { label: string; value: unknown; onChange: (value: number) => void; min?: number; max?: number }) { return <Field label={label}><input type="number" min={min} max={max} className={inputClass} value={typeof value === 'number' ? value : 1} onChange={(e) => onChange(Number(e.target.value))} /></Field> }
type VariableEntry = { name: string; type: string }
const VariableContext = createContext<VariableEntry[]>([])
function nodeVariables(node: CanvasNode): VariableEntry[] {
  const prefix = `${node.id}.`
  if (node.data.kind === 'condition') return []
  if (node.data.kind === 'loop') return list(obj(node.data.loopConfig).outputs).filter((item) => str(item.name).trim()).map((item) => ({ name: `${prefix}${str(item.name).trim()}`, type: 'any' }))
  if (node.data.kind === 'start') return [{ name: `${prefix}input`, type: 'string' }, ...list(node.data.inputVariables).filter((item) => str(item.name).trim()).map((item) => ({ name: `${prefix}${str(item.name).trim()}`, type: str(item.valueType) || 'any' }))]
  if (node.data.kind === 'iteration') return [{ name: `${prefix}output`, type: 'array[any]' }, { name: `${prefix}entries`, type: 'array[object]' }, { name: `${prefix}failed_count`, type: 'number' }]
  const entries = [{ name: `${prefix}output`, type: node.data.kind === 'aggregator' ? 'any' : 'string' }]
  const contract = obj(obj(node.data.agentConfig).outputContract)
  if (node.data.kind === 'agent' && contract.type === 'structured') {
    entries.push({ name: `${prefix}structured_output`, type: 'object' })
    const append = (schema: Obj, path: string[]) => { for (const [name, value] of Object.entries(obj(schema.properties))) { const property = obj(value); const next = [...path, name]; const type = str(property.type) || 'any'; entries.push({ name: `${prefix}structured_output.${next.join('.')}`, type }); if (type === 'object') append(property, next) } }
    append(obj(contract.schema), [])
  }
  return entries
}
function declaredType(name: string, nodes: CanvasNode[], globals: Obj[], seen = new Set<string>()): string {
  if (seen.has(name)) return 'any'
  const nextSeen = new Set(seen).add(name)
  const global = globals.find((item) => item.name === name)
  if (global) return str(global.valueType) || 'any'
  if (name === 'sys.timestamp') return 'number'
  if (name === 'sys.workflow_id') return 'string'
  const [id, root, ...path] = name.split('.')
  const node = nodes.find((item) => item.id === id)
  if (!node) return 'any'
  if (node.data.kind === 'start') return root === 'input' ? 'string' : str(list(node.data.inputVariables).find((item) => item.name === root)?.valueType) || 'any'
  if (node.data.kind === 'loop') { const variable = list(obj(node.data.loopConfig).variables).find((item) => item.name === root); if (variable) return str(variable.valueType) || 'any'; const output = list(obj(node.data.loopConfig).outputs).find((item) => item.name === root); return output ? declaredType(selectorText(output.variableSelector), nodes, globals, nextSeen) : 'any' }
  if (node.data.kind === 'aggregator' && root === 'output') {
    const values = obj(node.data.aggregatorConfig).variables
    const types = Array.isArray(values) ? values.map((value) => declaredType(selectorText(value), nodes, globals, nextSeen)) : []
    return types.length && types.every((type) => type === types[0]) ? types[0] : 'any'
  }
  if (node.data.kind === 'iteration') {
    if (root === 'index' || root === 'failed_count') return 'number'
    if (root === 'entries') return 'array[object]'
    if (root === 'item') { const sourceType = declaredType(selectorText(obj(node.data.iterationConfig).iteratorSelector), nodes, globals, nextSeen); return /^array\[(.+)\]$/.exec(sourceType)?.[1] || 'any' }
    if (root === 'output') { const collected = declaredType(selectorText(obj(node.data.iterationConfig).collectSelector), nodes, globals, nextSeen); return `array[${collected}]` }
  }
  if (root === 'structured_output') {
    let schema = obj(obj(obj(node.data.agentConfig).outputContract).schema)
    for (const part of path) schema = obj(obj(schema.properties)[part])
    if (schema.type === 'array') { const itemType = str(obj(schema.items).type); return ['string', 'number', 'object', 'boolean', 'file', 'any'].includes(itemType) ? `array[${itemType}]` : 'array' }
    return str(schema.type) || 'object'
  }
  return root === 'output' ? 'string' : 'any'
}
function variableCatalog(nodes: CanvasNode[], edges: Edge[], consumer: CanvasNode, globals: Obj[]): VariableEntry[] {
  const entries: VariableEntry[] = [{ name: 'sys.workflow_id', type: 'string' }, { name: 'sys.timestamp', type: 'number' }, ...globals.filter((item) => str(item.name).split('.').filter(Boolean).length >= 2).map((item) => ({ name: str(item.name), type: str(item.valueType) || 'any' }))]
  const byId = new Map(nodes.map((item) => [item.id, item]))
  const ancestors = new Set<string>()
  const visit = (id: string) => { if (id === consumer.id || ancestors.has(id) || !byId.has(id)) return; ancestors.add(id); edges.filter((edge) => edge.target === id).forEach((edge) => visit(edge.source)) }
  edges.filter((edge) => edge.target === consumer.id).forEach((edge) => visit(edge.source))
  const parent = consumer.parentId ? byId.get(consumer.parentId) : undefined
  const owner = parent && ['iteration', 'loop'].includes(str(parent.data.kind)) ? parent.id : null
  if (parent?.data.kind === 'loop') for (const variable of list(obj(parent.data.loopConfig).variables)) if (str(variable.name)) entries.push({ name: `${parent.id}.${str(variable.name)}`, type: str(variable.valueType) || 'any' })
  for (const item of nodes) {
    if (item.data.kind === 'iteration' && owner === item.id) {
      entries.push({ name: `${item.id}.item`, type: 'any' }, { name: `${item.id}.index`, type: 'number' })
      continue
    }
    if (!ancestors.has(item.id)) continue
    const itemParent = item.parentId ? byId.get(item.parentId) : undefined
    const itemOwner = itemParent && ['iteration', 'loop'].includes(str(itemParent.data.kind)) ? itemParent.id : null
    if (itemOwner && itemOwner !== owner) continue
    entries.push(...nodeVariables(item))
  }
  if (consumer.data.kind === 'loop') {
    for (const child of nodes.filter((item) => item.parentId === consumer.id)) entries.push(...nodeVariables(child))
    for (const variable of list(obj(consumer.data.loopConfig).variables)) if (str(variable.name)) entries.push({ name: `${consumer.id}.${str(variable.name)}`, type: str(variable.valueType) || 'any' })
  }
  return entries.filter((item, index) => entries.findIndex((candidate) => candidate.name === item.name) === index).map((item) => ({ ...item, type: declaredType(item.name, nodes, globals) }))
}
function SelectorField({ label, value, onChange, choices }: { label: string; value: unknown; onChange: (value: string[]) => void; nodes: CanvasNode[]; globalVariables?: Obj[]; choices?: VariableEntry[] }) {
  const id = useId(); const catalog = useContext(VariableContext)
  return <Field label={label}><input className={inputClass} value={selectorText(value)} onChange={(e) => onChange(selector(e.target.value))} placeholder="节点 ID.变量名" list={id}/><datalist id={id}>{(choices ?? catalog).map((item) => <option key={item.name} value={item.name}/>)}</datalist></Field>
}
function PromptField({ value, onChange }: { value: unknown; onChange: (value: string) => void; nodes: CanvasNode[]; globalVariables: Obj[] }) {
  const input = useRef<HTMLTextAreaElement>(null); const catalog = useContext(VariableContext)
  const [selected, setSelected] = useState('')
  const prompt = str(value)
  return <div className="space-y-1"><Field label="自定义 Prompt"><textarea ref={input} className={`${inputClass} min-h-28`} value={prompt} onChange={(e) => onChange(e.target.value)}/></Field><div className="flex gap-2"><select aria-label="插入变量" className={inputClass} value={selected} onChange={(e) => setSelected(e.target.value)}><option value="">选择变量</option>{catalog.map((item) => <option key={item.name} value={item.name}>{item.name}</option>)}</select><button type="button" className="shrink-0 rounded border px-2 text-xs" disabled={!selected} onClick={() => { const start = input.current?.selectionStart ?? prompt.length; const end = input.current?.selectionEnd ?? start; const token = `{{#${selected}#}}`; onChange(prompt.slice(0, start) + token + prompt.slice(end)); input.current?.focus() }}>插入</button></div></div>
}
function Section({ title, children, add }: { title: string; children: React.ReactNode; add?: () => void }) { return <section className="space-y-2 border-t pt-3"><div className="flex items-center justify-between text-xs font-semibold"><span>{title}</span>{add && <button type="button" className="rounded border px-2 py-1" onClick={add}>添加</button>}</div>{children}</section> }
function Item({ children, remove }: { children: React.ReactNode; remove: () => void }) { return <div className="space-y-2 rounded-md border p-2">{children}<button type="button" className="text-xs text-red-600" onClick={remove}>删除</button></div> }
function Bindings({ value, onChange, nodes, globalVariables, title = '输出变量' }: { value: unknown; onChange: (v: Obj[]) => void; nodes: CanvasNode[]; globalVariables?: Obj[]; title?: string }) {
  const rows = list(value); const update = (i: number, patch: Obj) => onChange(rows.map((row, n) => n === i ? { ...row, ...patch } : row))
  return <Section title={title} add={() => onChange([...rows, { name: '', variableSelector: [] }])}>{rows.map((row, i) => <Item key={i} remove={() => onChange(rows.filter((_, n) => n !== i))}><Text label="名称" value={row.name} onChange={(name) => update(i, { name })}/><SelectorField label="变量" value={row.variableSelector} nodes={nodes} globalVariables={globalVariables} onChange={(variableSelector) => update(i, { variableSelector })}/></Item>)}</Section>
}
function legacyOperator(operator: unknown, negated: unknown): unknown {
  const value = operator === 'is_empty' ? 'empty' : operator === 'is_not_empty' ? 'not_empty' : operator
  return negated === true ? ({ equals: 'not_equals', contains: 'not_contains', empty: 'not_empty' } as Obj)[str(value)] ?? value : value
}
function variableType(value: unknown, nodes: CanvasNode[], globals: Obj[]): string {
  const path = Array.isArray(value) ? value.map(str) : []
  const global = globals.find((item) => item.name === path.join('.'))
  if (global) return str(global.valueType) || 'string'
  if (path[0] === 'sys') return path[1] === 'timestamp' ? 'number' : 'string'
  const node = nodes.find((item) => item.id === path[0])
  if (node?.data.kind === 'start') return str(list(node.data.inputVariables).find((item) => item.name === path[1])?.valueType) || 'string'
  if (node?.data.kind === 'loop') return str(list(obj(node.data.loopConfig).variables).find((item) => item.name === path[1])?.valueType) || 'string'
  return 'string'
}
function Conditions({ value, onChange, nodes, globalVariables }: { value: unknown; onChange: (v: Obj[]) => void; nodes: CanvasNode[]; globalVariables?: Obj[] }) {
  const catalog = useContext(VariableContext)
  const rows = list(value); const update = (i: number, patch: Obj) => onChange(rows.map((row, n) => n === i ? { ...row, ...patch } : row))
  return <Section title="条件" add={() => onChange([...rows, { variableSelector: [], operator: 'equals', value: '' }])}>{rows.map((row, i) => <Item key={i} remove={() => onChange(rows.filter((_, n) => n !== i))}><SelectorField label="变量" value={row.variableSelector} nodes={nodes} globalVariables={globalVariables} onChange={(variableSelector) => update(i, { variableSelector })}/><Choice label="比较" value={row.operator ?? 'equals'} options={[[ 'equals','等于' ],[ 'not_equals','不等于' ],[ 'contains','包含' ],[ 'not_contains','不包含' ],[ 'starts_with','开头是' ],[ 'ends_with','结尾是' ],[ 'greater_than','大于' ],[ 'greater_than_or_equal','大于等于' ],[ 'less_than','小于' ],[ 'less_than_or_equal','小于等于' ],[ 'empty','为空' ],[ 'not_empty','不为空' ],[ 'exists','存在' ],[ 'not_exists','不存在' ]]} onChange={(operator) => update(i, { operator, value: undefined })}/>{!['empty', 'not_empty', 'exists', 'not_exists'].includes(str(row.operator)) && <TypedValue key={`${i}-${selectorText(row.variableSelector)}-${str(row.operator)}-${JSON.stringify(row.value)}`} label="值" value={row.value} valueType={catalog.find((item) => item.name === selectorText(row.variableSelector))?.type ?? variableType(row.variableSelector, nodes, globalVariables ?? [])} onChange={(value) => update(i, { value })}/>}</Item>)}</Section>
}

/** Purpose-specific editors keep desktop's graph data shape and untouched extension fields. */
export function WorkflowNodeInspector({ node, nodes = [], edges = [], globalVariables: rawGlobals, onChange, onClose, readOnly = false }: Props) {
  const globalVariables = list(rawGlobals)
  const catalog = variableCatalog(nodes, edges, node, globalVariables)
  const data = node.data; const kind = str(data.kind)
  const patch = (values: Obj) => onChange({ ...node, data: { ...data, ...values } })
  const nested = (key: string, values: Obj) => patch({ [key]: { ...obj(data[key]), ...values } })
  const startRows = list(data.inputVariables)
  const legacyCases = list(data.conditionBranches).map((branch, index) => ({ id: `case-${index + 1}`, logic: branch.logic ?? 'and', conditions: list(branch.conditions).map((condition) => ({ variableSelector: selector(str(condition.variable)), operator: legacyOperator(condition.operator, condition.negated), ...(condition.value !== '' ? { value: condition.value } : {}) })) }))
  const cases = data.cases !== undefined ? list(data.cases) : data.conditionCases !== undefined ? list(data.conditionCases) : legacyCases.length ? legacyCases : [{ id: 'case-1', logic: 'and', conditions: [] }]
  const setCases = (next: Obj[]) => { const { conditionCases: _cases, conditionBranches: _branches, ...rest } = data; onChange({ ...node, data: { ...rest, cases: next } }) }
  const aggregate = obj(data.aggregatorConfig); const selectors = Array.isArray(aggregate.variables) ? aggregate.variables : []
  const loop = obj(data.loopConfig); const loopVars = list(loop.variables); const until = obj(loop.until)
  const iteration = obj(data.iterationConfig); const agent = obj(data.agentConfig); const executor = obj(agent.executor)
  const retry = agent.retry === undefined ? { enabled: true, maxRetries: 2, initialDelaySeconds: 10 } : obj(agent.retry)
  return <><div className="flex items-center justify-between gap-2 border-b pb-3"><strong>节点配置</strong><button type="button" aria-label="关闭节点详情" onClick={onClose}>✕</button></div><VariableContext.Provider value={catalog}><fieldset key={node.id} disabled={readOnly} className="space-y-3 py-3 disabled:opacity-70">
    <Text label="名称" value={data.title} onChange={(title) => patch({ title })}/><Text label="描述" value={data.description} onChange={(description) => patch({ description })}/>
    {kind === 'start' && <><Text label="初始 Prompt" value={data.input} onChange={(input) => patch({ input })} multiline/><Section title="输入变量" add={() => patch({ inputVariables: [...startRows, { name: '', displayName: '', fieldType: 'text-input', valueType: 'string', required: false }] })}>{startRows.map((row, i) => { const update = (values: Obj) => patch({ inputVariables: startRows.map((item, n) => n === i ? { ...item, ...values } : item) }); return <Item key={i} remove={() => patch({ inputVariables: startRows.filter((_, n) => n !== i) })}><Text label="变量名" value={row.name} onChange={(name) => update({ name })}/><Text label="显示名称" value={row.displayName} onChange={(displayName) => update({ displayName })}/><Choice label="输入控件" value={row.fieldType ?? 'text-input'} options={['text-input','paragraph','select','number','checkbox','file','file-list','json'].map((v) => [v,v])} onChange={(fieldType) => update({ fieldType, valueType: ({ 'text-input': 'string', paragraph: 'string', select: 'string', number: 'number', checkbox: 'boolean', file: 'file', 'file-list': 'array[file]', json: 'object' } as Record<string, string>)[fieldType], value: undefined })}/><Choice label="值类型" value={row.valueType ?? 'string'} options={types.map((v) => [v,v])} onChange={(valueType) => update({ valueType })}/><TypedValue key={`${i}-${str(row.valueType)}-${JSON.stringify(row.value)}`} value={row.value} valueType={row.valueType} onChange={(value) => update({ value })}/><Field label="最大长度（可选）"><input type="number" min="1" className={inputClass} value={typeof row.maxLength === 'number' ? row.maxLength : ''} onChange={(e) => update({ maxLength: e.target.value ? Number(e.target.value) : undefined })}/></Field><label className="flex gap-2 text-xs"><input type="checkbox" checked={row.required === true} onChange={(e) => update({ required: e.target.checked })}/>必填</label>{row.fieldType === 'select' && <Text label="选项（每行一个）" value={Array.isArray(row.options) ? row.options.join('\n') : ''} multiline onChange={(value) => update({ options: value.split('\n') })}/>}</Item> })}</Section></>}
    {kind === 'agent' && <><Text label="Agent CLI" value={executor.agentCli} onChange={(agentCli) => nested('agentConfig', { executor: { ...executor, agentCli } })}/><Text label="模型 ID" value={executor.modelId} onChange={(modelId) => nested('agentConfig', { executor: { ...executor, modelId } })}/><Text label="角色 ID" value={agent.roleId} onChange={(roleId) => nested('agentConfig', { roleId })}/><PromptField value={agent.prompt} nodes={nodes} globalVariables={globalVariables} onChange={(prompt) => nested('agentConfig', { prompt })}/>{(['skills', 'mcps'] as const).map((key) => { const rows = list(agent[key]); const idKey = key === 'skills' ? 'skillId' : 'mcpId'; return <Section key={key} title={key === 'skills' ? 'Skills' : 'MCP'} add={() => nested('agentConfig', { [key]: [...rows, { [idKey]: '', enabled: true }] })}>{rows.map((row, i) => { const update = (values: Obj) => nested('agentConfig', { [key]: rows.map((item, n) => n === i ? { ...item, ...values } : item) }); return <Item key={i} remove={() => nested('agentConfig', { [key]: rows.filter((_, n) => n !== i) })}><Text label={key === 'skills' ? 'Skill ID' : 'MCP ID'} value={row[idKey]} onChange={(value) => update({ [idKey]: value })}/><label className="flex gap-2 text-xs"><input type="checkbox" checked={row.enabled !== false} onChange={(e) => update({ enabled: e.target.checked })}/>启用</label></Item> })}</Section> })}<label className="flex gap-2 text-xs"><input type="checkbox" checked={agent.interactive === true} onChange={(e) => nested('agentConfig', { interactive: e.target.checked })}/>交互式</label><Choice label="提示等待策略" value={agent.promptInactivity ?? ''} options={[[ '', '默认' ], [ 'timeout', '超时' ], [ 'wait', '等待' ]]} onChange={(promptInactivity) => nested('agentConfig', { promptInactivity: promptInactivity || null })}/><Section title="结构化输出"><label className="flex gap-2 text-xs"><input type="checkbox" checked={obj(agent.outputContract).type === 'structured'} onChange={(e) => nested('agentConfig', { outputContract: e.target.checked ? { type: 'structured', schema: { type: 'object', properties: {}, required: [], additionalProperties: false } } : undefined })}/>启用结构化输出</label>{obj(agent.outputContract).type === 'structured' && <SchemaEditor key={JSON.stringify(obj(agent.outputContract).schema)} value={obj(agent.outputContract).schema} onChange={(schema) => nested('agentConfig', { outputContract: { ...obj(agent.outputContract), schema } })}/>}</Section><Section title="自动重试"><label className="flex gap-2 text-xs"><input type="checkbox" checked={retry.enabled === true} onChange={(e) => nested('agentConfig', { retry: { ...retry, enabled: e.target.checked } })}/>启用重试</label>{retry.enabled === true && <><NumberField label="重试次数" min={0} max={5} value={retry.maxRetries} onChange={(maxRetries) => nested('agentConfig', { retry: { ...retry, maxRetries } })}/><NumberField label="初始等待秒数" min={0} max={300} value={retry.initialDelaySeconds} onChange={(initialDelaySeconds) => nested('agentConfig', { retry: { ...retry, initialDelaySeconds } })}/></>}</Section><p className="text-xs text-muted-foreground">Agent 执行与模型选择尚未在 Cloud v2 开放；配置会随草稿保存。</p></>}
    {kind === 'condition' && <><Section title="分支" add={() => { let n = 1; while (cases.some((row) => row.id === `case-${n}`)) n++; setCases([...cases, { id: `case-${n}`, logic: 'and', conditions: [] }]) }}>{cases.map((row, i) => { const update = (values: Obj) => setCases(cases.map((item, n) => n === i ? { ...item, ...values } : item)); return <div key={str(row.id) || i} className="space-y-2 rounded-md border p-2"><strong className="text-xs">{i === 0 ? 'IF' : 'ELIF'} · {str(row.id)}</strong><Choice label="条件关系" value={row.logic ?? 'and'} options={[[ 'and','全部满足 (AND)' ],[ 'or','任一满足 (OR)' ]]} onChange={(logic) => update({ logic })}/><Conditions value={row.conditions} nodes={nodes} globalVariables={globalVariables} onChange={(conditions) => update({ conditions })}/>{i > 0 && <button type="button" className="text-xs text-red-600" onClick={() => setCases(cases.filter((_, n) => n !== i))}>删除分支</button>}</div> })}</Section><p className="text-xs">ELSE：以上条件均不满足时</p></>}
    {kind === 'aggregator' && <Section title="候选变量（按顺序）" add={() => nested('aggregatorConfig', { variables: [...selectors, []] })}>{selectors.map((value, i) => <Item key={i} remove={() => nested('aggregatorConfig', { variables: selectors.filter((_, n) => n !== i) })}><SelectorField label={`变量 ${i + 1}`} value={value} nodes={nodes} globalVariables={globalVariables} onChange={(next) => nested('aggregatorConfig', { variables: selectors.map((item, n) => n === i ? next : item) })}/><div className="flex gap-2 text-xs"><button type="button" disabled={i === 0} onClick={() => { const next = [...selectors]; [next[i-1], next[i]] = [next[i], next[i-1]]; nested('aggregatorConfig', { variables: next }) }}>上移</button><button type="button" disabled={i === selectors.length - 1} onClick={() => { const next = [...selectors]; [next[i+1], next[i]] = [next[i], next[i+1]]; nested('aggregatorConfig', { variables: next }) }}>下移</button></div></Item>)}</Section>}
    {kind === 'loop' && <><NumberField label="最大循环次数" value={loop.maxIterations} onChange={(maxIterations) => nested('loopConfig', { maxIterations })}/><Choice label="达到上限时" value={loop.maxIterationsBehavior ?? 'fail'} options={[[ 'fail','失败' ],[ 'succeed','成功结束' ]]} onChange={(maxIterationsBehavior) => nested('loopConfig', { maxIterationsBehavior })}/><Section title="循环变量" add={() => nested('loopConfig', { variables: [...loopVars, { name: '', valueType: 'string', initial: { kind: 'constant', value: '' }, feedback: [] }] })}>{loopVars.map((row, i) => { const update = (values: Obj) => nested('loopConfig', { variables: loopVars.map((item, n) => n === i ? { ...item, ...values } : item) }); const initial = obj(row.initial); return <Item key={i} remove={() => nested('loopConfig', { variables: loopVars.filter((_, n) => n !== i) })}><Text label="名称" value={row.name} onChange={(name) => update({ name })}/><Choice label="值类型" value={row.valueType ?? 'string'} options={types.map((v) => [v,v])} onChange={(valueType) => update({ valueType })}/><Choice label="初始值来源" value={initial.kind ?? 'constant'} options={[[ 'constant','常量' ],[ 'variable','变量' ]]} onChange={(kind) => update({ initial: kind === 'variable' ? { kind, selector: [] } : { kind, value: '' } })}/>{initial.kind === 'variable' ? <SelectorField label="初始变量" value={initial.selector} nodes={nodes} globalVariables={globalVariables} onChange={(selector) => update({ initial: { ...initial, selector } })}/> : <TypedValue key={`${i}-${str(row.valueType)}-${JSON.stringify(initial.value)}`} value={initial.value} valueType={row.valueType} onChange={(value) => update({ initial: { ...initial, value } })}/>}<SelectorField label="反馈变量" value={row.feedback} nodes={nodes} globalVariables={globalVariables} onChange={(feedback) => update({ feedback })}/></Item> })}</Section><Choice label="结束条件关系" value={until.logic ?? 'and'} options={[[ 'and','全部满足' ],[ 'or','任一满足' ]]} onChange={(logic) => nested('loopConfig', { until: { ...until, logic } })}/><Conditions value={until.conditions} nodes={nodes} globalVariables={globalVariables} onChange={(conditions) => nested('loopConfig', { until: { ...until, conditions } })}/><Bindings value={loop.outputs} nodes={nodes} globalVariables={globalVariables} onChange={(outputs) => nested('loopConfig', { outputs })}/></>}
    {kind === 'loopExit' && <p className="text-xs text-muted-foreground">到达此节点时退出所在循环。</p>}
    {kind === 'iteration' && <><SelectorField choices={catalog.filter((item) => item.type.startsWith('array'))} label="迭代数组" value={iteration.iteratorSelector} nodes={nodes} globalVariables={globalVariables} onChange={(iteratorSelector) => nested('iterationConfig', { iteratorSelector })}/><SelectorField choices={nodes.filter((item) => item.parentId === node.id).flatMap(nodeVariables).filter((item) => item.name.endsWith('.output') || item.name.endsWith('.structured_output'))} label="每轮收集变量" value={iteration.collectSelector} nodes={nodes} globalVariables={globalVariables} onChange={(collectSelector) => nested('iterationConfig', { collectSelector })}/><NumberField label="最大迭代次数" value={iteration.maxIterations} onChange={(maxIterations) => nested('iterationConfig', { maxIterations })}/><Choice label="错误处理" value={iteration.errorStrategy ?? 'fail'} options={[[ 'fail','停止并失败' ],[ 'continue','继续' ]]} onChange={(errorStrategy) => nested('iterationConfig', { errorStrategy })}/></>}
    {kind === 'output' && <Bindings value={data.outputs} nodes={nodes} globalVariables={globalVariables} onChange={(outputs) => patch({ outputs })}/>}
  </fieldset></VariableContext.Provider></>
}
