import { useGetApiV1TenantsTidMembers } from '@/api/members/members'
import { useContractsClient } from '@/lib/workflows/data/contracts-client'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import type { Node } from '@xyflow/react'
import type { WorkflowNodeData, WorkflowVariableCatalogEntry } from '@/lib/workflows/mock'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'

/** Configuration shared by newly authored inspection reviews. */
type ReviewField = {name:string;label:string;type:string;required:boolean;options?:string[]}
const defaultReviewConfig = {
  title: '巡检问题确认', submitLabel: '确认并生成任务', reviewerId: '',
  reportSelector: ['report', 'structured_output', 'reportMarkdown'],
  itemsSelector: ['report', 'structured_output', 'findings'],
  fields: [{ name: 'notes', label: '备注', type: 'textarea', required: false }] as ReviewField[],
}

/** Edits frozen review bindings; task effects remain a separate successor node. */
export function WorkflowFormFields({ node, onUpdate, onClose, variableCatalog, graphNodes = [] }: { variableCatalog: WorkflowVariableCatalogEntry[]; graphNodes?: Node<WorkflowNodeData, 'workflow'>[]; node: Node<WorkflowNodeData, 'workflow'>; onUpdate: (node: Node<WorkflowNodeData, 'workflow'>) => void; onClose: () => void }) {
  const tenantId=useContractsClient().tenantId
  const members=useGetApiV1TenantsTidMembers(tenantId,undefined,{query:{enabled:!!tenantId}})
  const isForm = node.data.kind === 'form'
  const config = { ...defaultReviewConfig, ...(node.data.formConfig as Partial<typeof defaultReviewConfig> | undefined) }
  const task = (node.data.createTasksConfig ?? { formNodeId: '' }) as { formNodeId: string }
  const update = (patch: Partial<typeof defaultReviewConfig>) => onUpdate({ ...node, data: { ...node.data, formConfig: { ...config, ...patch } } })
  return <section className="flex h-full flex-col gap-5 overflow-auto p-4" aria-label={isForm ? '动态表单配置' : '创建任务配置'}>
    <h2 className="font-semibold">{isForm ? '动态表单' : '创建任务'}</h2>
    <FieldGroup>
      <Field><FieldLabel htmlFor="review-node-title">节点名称</FieldLabel><Input id="review-node-title" value={node.data.title} onChange={e => onUpdate({ ...node, data: { ...node.data, title: e.target.value } })} /></Field>
      {isForm ? <>
        <Field><FieldLabel htmlFor="review-title">表单标题</FieldLabel><Input id="review-title" value={config.title} onChange={e => update({ title: e.target.value })} /></Field>
        <Field><FieldLabel htmlFor="review-report">报告来源变量</FieldLabel><SourceSelect id="review-report" value={config.reportSelector} entries={variableCatalog.filter(v=>v.valueType==='string'||v.valueType==='any')} onChange={reportSelector=>update({reportSelector})} /></Field>
        <Field><FieldLabel htmlFor="review-items">问题列表来源变量</FieldLabel><SourceSelect id="review-items" value={config.itemsSelector} entries={variableCatalog.filter(v=>v.valueType.startsWith('array')||v.valueType==='any')} onChange={itemsSelector=>update({itemsSelector})} /></Field>
        <Field><FieldLabel htmlFor="review-user">确认人</FieldLabel><NativeSelect id="review-user" value={config.reviewerId} onChange={e => update({ reviewerId: e.target.value })}><NativeSelectOption value="">运行发起人</NativeSelectOption>{(members.data?.items??[]).filter(m=>m.status==='active').map(m=><NativeSelectOption key={m.userId} value={m.userId}>{m.displayName}</NativeSelectOption>)}</NativeSelect></Field>
        <Field><FieldLabel htmlFor="review-submit">提交按钮文案</FieldLabel><Input id="review-submit" value={config.submitLabel} onChange={e => update({ submitLabel: e.target.value })} /></Field>
        <p className="text-sm text-muted-foreground">每行显示问题与证据，收集确认结果、负责人和备注。确认结果交给后续节点处理。</p>
        {config.fields.map((field,index)=><Field key={index}><FieldLabel>附加字段 {index+1}</FieldLabel>
          <Input aria-label={`字段 ${index+1} 标识`} value={field.name} onChange={e=>update({fields:config.fields.map((f,i)=>i===index?{...f,name:e.target.value}:f)})}/>
          <Input aria-label={`字段 ${index+1} 名称`} value={field.label} onChange={e=>update({fields:config.fields.map((f,i)=>i===index?{...f,label:e.target.value}:f)})}/>
          <NativeSelect aria-label={`字段 ${index+1} 类型`} value={field.type} onChange={e=>update({fields:config.fields.map((f,i)=>i===index?{...f,type:e.target.value,options:e.target.value==='select'?['选项一','选项二']:undefined}:f)})}>
            {[['text','文本'],['textarea','多行文本'],['select','下拉选择'],['readonly','只读文本']].map(([v,label])=><NativeSelectOption key={v} value={v}>{label}</NativeSelectOption>)}
          </NativeSelect>
          {field.type==='select'&&<Input aria-label={`字段 ${index+1} 选项`} value={(field.options??[]).join('，')} onChange={e=>update({fields:config.fields.map((f,i)=>i===index?{...f,options:e.target.value.split(/[，,]/)}:f)})}/>}
          <label className="flex items-center gap-2"><input type="checkbox" checked={field.required} onChange={e=>update({fields:config.fields.map((f,i)=>i===index?{...f,required:e.target.checked}:f)})}/>必填</label>
          <Button variant="outline" onClick={()=>update({fields:config.fields.filter((_,i)=>i!==index)})}>移除字段</Button>
        </Field>)}
        <Button variant="outline" disabled={config.fields.length>=20} onClick={()=>update({fields:[...config.fields,{name:`field${config.fields.length+1}`,label:'新字段',type:'text',required:false}]})}>添加字段</Button>
        <Button variant="outline" onClick={() => update(config)}>应用表单配置</Button>
      </> : <>
        <Field><FieldLabel htmlFor="review-source">上游表单</FieldLabel><NativeSelect id="review-source" value={task.formNodeId} onChange={e => onUpdate({ ...node, data: { ...node.data, createTasksConfig: { formNodeId: e.target.value } } })}><NativeSelectOption value="">请选择表单节点</NativeSelectOption>{task.formNodeId&&!graphNodes.some(n=>n.id===task.formNodeId)&&<NativeSelectOption value={task.formNodeId}>原绑定（节点不可用）</NativeSelectOption>}{graphNodes.filter(n=>n.data.kind==='form').map(n=><NativeSelectOption key={n.id} value={n.id}>{n.data.title}</NativeSelectOption>)}</NativeSelect></Field>
        <p className="text-sm text-muted-foreground">仅处理确认的问题，复用未关闭任务，保持其负责人。新任务沿用本次运行的项目。</p>
      </>}
    </FieldGroup>
    <Button variant="outline" onClick={onClose}>关闭配置</Button>
  </section>
}

function SourceSelect({id,value,entries,onChange}:{id:string;value:string[];entries:WorkflowVariableCatalogEntry[];onChange:(selector:string[])=>void}){
 const selected=JSON.stringify(value)
 return <NativeSelect id={id} value={selected} onChange={e=>onChange(entries.find(v=>JSON.stringify(v.selector)===e.target.value)?.selector??[])}>
  <NativeSelectOption value="[]">请选择上游变量</NativeSelectOption>
  {value.length>0&&!entries.some(v=>JSON.stringify(v.selector)===selected)&&<NativeSelectOption value={selected}>原绑定：{value.join('.')}（请检查连线）</NativeSelectOption>}
  {entries.map(v=><NativeSelectOption key={JSON.stringify(v.selector)} value={JSON.stringify(v.selector)}>{v.sourceNodeTitle||v.sourceNodeId} · {v.variableName}</NativeSelectOption>)}
 </NativeSelect>
}
