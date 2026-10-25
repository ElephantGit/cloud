import { useTranslation } from 'react-i18next'
import { Plus, Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import {
  createDefaultWorkflowCapabilities,
  workflowChoiceLabel,
  type WorkflowChoice,
} from '@/features/workflows/runtime/capabilities'
import type {
  WorkflowGlobalVariable,
  WorkflowLoopVariable,
  WorkflowNodeData,
  WorkflowOutputBinding,
  WorkflowToolParameter,
} from '@/features/workflows/runtime/types'
import type { TranslationKey } from '@/i18n/i18n-instance'
import { InspectorField } from '@/features/workflows/editor/workflow-inspector-fields'
import { useWorkflowTranslator } from '@/features/workflows/editor/use-workflow-translator'
import type { WorkflowCanvasNode } from '@/features/workflows/editor/canvas-types'
import type { WorkflowDecoratedVariable } from '@/features/workflows/runtime/variable-catalog'
import {
  LocalizedSelectValue,
  WorkflowVariableTypeMenu,
  comparisonValueToText,
  parseComparisonValue,
  selectorToText,
  textToSelector,
} from '@/features/workflows/editor/workflow-variable-select'

/**
 * Everything the kind-specific panels may read beyond the selected node itself.
 *
 * Only Condition and Iteration panels read the catalog and the graph; the rest ignore what
 * they do not need, so one shared props shape keeps the dispatcher and the panels aligned.
 */
export interface WorkflowKindFieldsProps {
  /** Id of the node the panel edits; the root of an Iteration region-membership lookup. */
  nodeId: string
  data: WorkflowNodeData
  onChange: (data: WorkflowNodeData) => void
  /** Variables the selected node may reference, globals first, already decorated. */
  catalog: readonly WorkflowDecoratedVariable[]
  /** Whole-graph nodes, for region-member lookups the Iteration panel needs. */
  graphNodes: readonly WorkflowCanvasNode[]
  /** Workflow-wide declarations, normalized to include the required system globals. */
  globalVariables: readonly WorkflowGlobalVariable[]
}

/** Edits a Tool node: the tool, its derived operation, and key/value parameters. */
export function ToolNodeFields({
  data,
  onChange,
}: Pick<WorkflowKindFieldsProps, 'data' | 'onChange'>) {
  const { t } = useTranslation()
  const translate = useWorkflowTranslator()
  const capabilities = createDefaultWorkflowCapabilities()
  const selectedTool = data.tool ?? capabilities.defaultTool
  const operations = capabilities.toolOperations[selectedTool] ?? []
  const firstOperation = operations[0]?.value
  return (
    <>
      <InspectorField label={t('workflows.tool.field.tool')} htmlFor="workflow-node-tool">
        <Select
          value={selectedTool}
          onValueChange={(tool) => {
            if (tool !== null) {
              const firstOperationOf = capabilities.toolOperations[tool]?.[0]?.value
              onChange({
                ...data,
                tool,
                // Switch to the first operation of the newly selected tool.
                ...(firstOperationOf === undefined ? {} : { operation: firstOperationOf }),
              })
            }
          }}
        >
          <SelectTrigger id="workflow-node-tool" className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {capabilities.tools.map((tool) => (
              <SelectItem key={tool.value} value={tool.value}>
                {workflowChoiceLabel(tool, translate)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </InspectorField>
      {operations.length > 0 ? (
        <InspectorField
          label={t('workflows.tool.field.operation')}
          htmlFor="workflow-node-operation"
        >
          <Select
            value={firstOperation === undefined ? '' : (data.operation ?? firstOperation)}
            onValueChange={(operation) => {
              if (operation !== null) {
                onChange({ ...data, operation })
              }
            }}
          >
            <SelectTrigger id="workflow-node-operation" className="w-full">
              <LocalizedSelectValue
                options={operations}
                value={firstOperation === undefined ? '' : (data.operation ?? firstOperation)}
              />
            </SelectTrigger>
            <SelectContent>
              {operations.map((operation) => (
                <SelectItem key={operation.value} value={operation.value}>
                  {workflowChoiceLabel(operation, translate)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </InspectorField>
      ) : (
        <p className="text-[11px] text-muted-foreground">{t('workflows.tool.noOperations')}</p>
      )}
      <ToolParametersField data={data} onChange={onChange} />
    </>
  )
}

/** The key/value call parameters of a Tool node. */
function ToolParametersField({
  data,
  onChange,
}: {
  data: WorkflowNodeData
  onChange: (data: WorkflowNodeData) => void
}) {
  const { t } = useTranslation()
  const toolParameters = data.toolParameters ?? []
  const updateParameters = (parameters: WorkflowToolParameter[]): void => {
    onChange({ ...data, toolParameters: parameters })
  }
  return (
    <InspectorField label={t('workflows.section.parameters')} htmlFor="workflow-tool-parameters">
      <div className="space-y-2" id="workflow-tool-parameters">
        {toolParameters.map((parameter, index) => (
          <div
            key={toolParameterKey(parameter)}
            className="grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto] items-center gap-2"
          >
            <Input
              value={parameter.key}
              aria-label={t('workflows.tool.parameterName', { index: index + 1 })}
              placeholder={t('workflows.tool.parameterName')}
              className="h-8"
              onChange={(event) =>
                updateParameters(
                  toolParameters.map((candidate, candidateIndex) =>
                    candidateIndex === index
                      ? { ...candidate, key: event.target.value }
                      : candidate,
                  ),
                )
              }
            />
            <Input
              value={parameter.value}
              aria-label={t('workflows.tool.parameterValue', { index: index + 1 })}
              placeholder={t('workflows.tool.parameterValue')}
              className="h-8"
              onChange={(event) =>
                updateParameters(
                  toolParameters.map((candidate, candidateIndex) =>
                    candidateIndex === index
                      ? { ...candidate, value: event.target.value }
                      : candidate,
                  ),
                )
              }
            />
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              className="shrink-0 text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
              aria-label={t('workflows.tool.removeParameter', { index: index + 1 })}
              onClick={() =>
                updateParameters(
                  toolParameters.filter((_, candidateIndex) => candidateIndex !== index),
                )
              }
            >
              <Trash2 className="size-3.5" />
            </Button>
          </div>
        ))}
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="w-full justify-start"
          onClick={() => updateParameters([...toolParameters, { key: '', value: '' }])}
        >
          <Plus />
          {t('workflows.tool.addParameter')}
        </Button>
      </div>
    </InspectorField>
  )
}

/** A select whose menu text and closed-state label both come from one choice list. */
function LocalizedChoiceSelect({
  id,
  label,
  value,
  options,
  disabled,
  onChange,
}: {
  id: string
  label: string
  value: string
  options: readonly WorkflowChoice[]
  disabled?: boolean
  onChange: (value: string | null) => void
}) {
  const translate = useWorkflowTranslator()
  return (
    <InspectorField label={label} htmlFor={id}>
      <Select value={value} disabled={disabled} onValueChange={onChange}>
        <SelectTrigger id={id} className="w-full">
          <LocalizedSelectValue options={options} value={value} />
        </SelectTrigger>
        <SelectContent>
          {options.map((option) => (
            <SelectItem key={option.value} value={option.value}>
              {workflowChoiceLabel(option, translate)}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </InspectorField>
  )
}

/** Edits the merge behavior of a Junction node: wait and failure strategies. */
export function JunctionNodeFields({
  data,
  onChange,
}: Pick<WorkflowKindFieldsProps, 'data' | 'onChange'>) {
  const { t } = useTranslation()
  return (
    <>
      <LocalizedChoiceSelect
        id="workflow-node-wait-strategy"
        label={t('workflows.junction.field.waitStrategy')}
        value={data.waitStrategy ?? 'all'}
        options={WORKFLOW_JUNCTION_WAIT_CHOICES}
        onChange={(strategy) => {
          if (strategy === 'all' || strategy === 'any' || strategy === 'count') {
            onChange({ ...data, waitStrategy: strategy })
          }
        }}
      />
      {data.waitStrategy === 'count' && (
        <InspectorField
          label={t('workflows.junction.field.waitCount')}
          htmlFor="workflow-node-wait-count"
        >
          <Input
            id="workflow-node-wait-count"
            type="number"
            min={1}
            value={data.waitCount ?? 1}
            onChange={(event) => {
              const parsed = Number(event.target.value)
              // Dropping the count removes the key rather than writing `undefined`,
              // which would trip `exactOptionalPropertyTypes` on the wire format.
              const next: WorkflowNodeData = { ...data }
              if (event.target.value !== '' && Number.isFinite(parsed)) {
                next.waitCount = parsed
              } else {
                delete next.waitCount
              }
              onChange(next)
            }}
          />
        </InspectorField>
      )}
      <LocalizedChoiceSelect
        id="workflow-node-failure-strategy"
        label={t('workflows.junction.field.failureStrategy')}
        value={data.failureStrategy ?? 'fail'}
        options={WORKFLOW_JUNCTION_FAILURE_CHOICES}
        onChange={(strategy) => {
          if (strategy === 'fail' || strategy === 'continue') {
            onChange({ ...data, failureStrategy: strategy })
          }
        }}
      />
    </>
  )
}

/** Junction wait strategy choices keyed to the graph's enum names. */
const WORKFLOW_JUNCTION_WAIT_CHOICES: readonly {
  value: 'all' | 'any' | 'count'
  labelKey: TranslationKey
}[] = [
  { value: 'all', labelKey: 'workflows.junction.waitAll' },
  { value: 'any', labelKey: 'workflows.junction.waitAny' },
  { value: 'count', labelKey: 'workflows.junction.waitCount' },
]

/** Junction failure strategy choices keyed to the graph's enum names. */
const WORKFLOW_JUNCTION_FAILURE_CHOICES: readonly {
  value: 'fail' | 'continue'
  labelKey: TranslationKey
}[] = [
  { value: 'fail', labelKey: 'workflows.junction.failFast' },
  { value: 'continue', labelKey: 'workflows.junction.collectResults' },
]

/** Edits the approval prompt a Human node shows to the reviewer. */
export function HumanNodeFields({
  data,
  onChange,
}: Pick<WorkflowKindFieldsProps, 'data' | 'onChange'>) {
  const { t } = useTranslation()
  return (
    <InspectorField
      label={t('workflows.human.field.approvalPrompt')}
      htmlFor="workflow-node-instruction"
    >
      <Textarea
        id="workflow-node-instruction"
        className="min-h-32 resize-none text-xs leading-5"
        value={data.instruction ?? ''}
        onChange={(event) => onChange({ ...data, instruction: event.target.value })}
      />
    </InspectorField>
  )
}

/** Exhaustion policies of a Loop, keyed to the graph's enum names. */
const WORKFLOW_LOOP_BEHAVIOR_CHOICES = [
  { value: 'fail', labelKey: 'workflows.loop.behavior.fail' },
  { value: 'succeed', labelKey: 'workflows.loop.behavior.succeed' },
] as const satisfies readonly WorkflowChoice[]

/** Edits a Loop container: budget, carried initial value, and the behavior note. */
export function LoopNodeFields({
  data,
  onChange,
}: Pick<WorkflowKindFieldsProps, 'data' | 'onChange'>) {
  const { t } = useTranslation()
  const loopConfig = data.loopConfig
  return (
    <>
      <LoopBudgetFields data={data} onChange={onChange} />
      <LoopVariablesFields data={data} onChange={onChange} />
      <LoopOutputsFields data={data} onChange={onChange} />
      <p className="rounded-lg border border-border bg-muted/25 px-3 py-2 text-[11px] leading-5 text-muted-foreground">
        {t(
          loopConfig === undefined
            ? 'workflows.loop.legacyUnsupported'
            : 'workflows.loop.defaultBehavior',
        )}
      </p>
    </>
  )
}

/** Edits the round ceiling and what an exhausted budget does. */
function LoopBudgetFields({ data, onChange }: Pick<WorkflowKindFieldsProps, 'data' | 'onChange'>) {
  const { t } = useTranslation()
  const loopConfig = data.loopConfig
  const updateMaxIterations = (value: number): void => {
    if (loopConfig === undefined) {
      return
    }
    onChange({
      ...data,
      loopConfig: {
        ...loopConfig,
        maxIterations: Math.min(100, Math.max(1, Math.trunc(value))),
      },
    })
  }
  const updateMaxIterationsBehavior = (value: string | null): void => {
    if (loopConfig === undefined || (value !== 'fail' && value !== 'succeed')) {
      return
    }
    onChange({
      ...data,
      loopConfig: { ...loopConfig, maxIterationsBehavior: value },
    })
  }
  return (
    <>
      <InspectorField
        label={t('workflows.loop.field.maxIterations')}
        htmlFor="workflow-node-max-iterations"
      >
        <Input
          id="workflow-node-max-iterations"
          type="number"
          min={1}
          max={100}
          value={loopConfig?.maxIterations ?? 3}
          disabled={loopConfig === undefined}
          onChange={(event) => {
            const parsed = Number(event.target.value)
            if (Number.isFinite(parsed)) {
              updateMaxIterations(parsed)
            }
          }}
        />
      </InspectorField>
      <LocalizedChoiceSelect
        id="workflow-node-max-iterations-behavior"
        label={t('workflows.loop.field.maxIterationsBehavior')}
        value={loopConfig?.maxIterationsBehavior ?? 'fail'}
        options={WORKFLOW_LOOP_BEHAVIOR_CHOICES}
        disabled={loopConfig === undefined}
        onChange={updateMaxIterationsBehavior}
      />
    </>
  )
}

/** Parses a typed constant: text stays text for string types, otherwise it is JSON-read. */
function loopConstantValue(text: string, valueType: string): unknown {
  return valueType === 'string' || valueType === 'secret' ? text : parseComparisonValue(text)
}

/** Builds the empty carried variable a new row starts from. */
function blankLoopVariable(): WorkflowLoopVariable {
  return { name: '', valueType: 'string', initial: { kind: 'constant', value: '' }, feedback: [] }
}

/** Edits every carried variable: its name, type, first-round value and feedback selector. */
function LoopVariablesFields({
  data,
  onChange,
}: Pick<WorkflowKindFieldsProps, 'data' | 'onChange'>) {
  const { t } = useTranslation()
  const loopConfig = data.loopConfig
  if (loopConfig === undefined) {
    return null
  }
  const update = (variables: WorkflowLoopVariable[]): void => {
    onChange({ ...data, loopConfig: { ...loopConfig, variables } })
  }
  return (
    <InspectorField label={t('workflows.loop.field.variables')} htmlFor="workflow-loop-variables">
      <div className="space-y-2" id="workflow-loop-variables">
        {/* oxlint-disable react/no-array-index-key -- a carried variable has no stable id; its name is the editable identity, so a content key collides on a blank new row. */}
        {loopConfig.variables.map((variable, index) => (
          <LoopVariableRow
            key={`loop-variable-${index}`}
            index={index}
            variable={variable}
            onChange={(next) =>
              update(loopConfig.variables.map((current, i) => (i === index ? next : current)))
            }
            onRemove={() => update(loopConfig.variables.filter((_, i) => i !== index))}
          />
        ))}
        {/* oxlint-enable react/no-array-index-key */}
      </div>
      <Button
        type="button"
        variant="outline"
        size="sm"
        onClick={() => update([...loopConfig.variables, blankLoopVariable()])}
      >
        <Plus />
        {t('workflows.loop.addVariable')}
      </Button>
    </InspectorField>
  )
}

/** One carried variable's fields; the index only labels the controls for assistive technology. */
function LoopVariableRow({
  index,
  variable,
  onChange,
  onRemove,
}: {
  index: number
  variable: WorkflowLoopVariable
  onChange: (variable: WorkflowLoopVariable) => void
  onRemove: () => void
}) {
  const { t } = useTranslation()
  const label = (key: TranslationKey): string => `${t(key)} ${index + 1}`
  return (
    <div className="space-y-2 rounded-lg border border-border p-2">
      <div className="flex items-center gap-2">
        <Input
          aria-label={label('workflows.loop.field.variableName')}
          value={variable.name}
          onChange={(event) => onChange({ ...variable, name: event.target.value })}
        />
        <Button
          type="button"
          variant="ghost"
          size="icon"
          aria-label={label('workflows.loop.removeVariable')}
          onClick={onRemove}
        >
          <Trash2 />
        </Button>
      </div>
      <div className="flex gap-2">
        <Select
          value={variable.valueType}
          onValueChange={(valueType) => {
            if (valueType !== null) {
              onChange({ ...variable, valueType })
            }
          }}
        >
          <SelectTrigger
            aria-label={label('workflows.loop.field.variableType')}
            className="h-8 w-full"
          >
            <SelectValue />
          </SelectTrigger>
          <WorkflowVariableTypeMenu />
        </Select>
        <Select
          value={variable.initial.kind}
          onValueChange={(kind) => {
            if (kind === 'constant') {
              onChange({ ...variable, initial: { kind: 'constant', value: '' } })
            } else if (kind === 'variable') {
              onChange({ ...variable, initial: { kind: 'variable', selector: [] } })
            }
          }}
        >
          <SelectTrigger
            aria-label={label('workflows.loop.field.variableInitialKind')}
            className="h-8 w-full"
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="constant">{t('workflows.loop.initialKind.constant')}</SelectItem>
            <SelectItem value="variable">{t('workflows.loop.initialKind.variable')}</SelectItem>
          </SelectContent>
        </Select>
      </div>
      <LoopInitialValueInput index={index} variable={variable} onChange={onChange} />
      <Input
        aria-label={label('workflows.loop.field.variableFeedback')}
        placeholder={t('workflows.loop.selectorPlaceholder')}
        value={selectorToText(variable.feedback)}
        onChange={(event) =>
          onChange({ ...variable, feedback: textToSelector(event.target.value) })
        }
      />
    </div>
  )
}

/**
 * The first-round value: a typed constant, or a reference resolved once before the loop starts.
 * Switching kinds replaces the value, because a constant and a selector cannot be converted into
 * each other without guessing what the author meant.
 */
function LoopInitialValueInput({
  index,
  variable,
  onChange,
}: {
  index: number
  variable: WorkflowLoopVariable
  onChange: (variable: WorkflowLoopVariable) => void
}) {
  const { t } = useTranslation()
  const label = `${t('workflows.loop.field.variableInitial')} ${index + 1}`
  if (variable.initial.kind === 'constant') {
    return (
      <Input
        aria-label={label}
        value={comparisonValueToText(variable.initial.value)}
        onChange={(event) =>
          onChange({
            ...variable,
            initial: {
              kind: 'constant',
              value: loopConstantValue(event.target.value, variable.valueType),
            },
          })
        }
      />
    )
  }
  return (
    <Input
      aria-label={label}
      placeholder={t('workflows.loop.selectorPlaceholder')}
      value={selectorToText(variable.initial.selector)}
      onChange={(event) =>
        onChange({
          ...variable,
          initial: { kind: 'variable', selector: textToSelector(event.target.value) },
        })
      }
    />
  )
}

/** Edits the named results the loop publishes when it succeeds. */
function LoopOutputsFields({ data, onChange }: Pick<WorkflowKindFieldsProps, 'data' | 'onChange'>) {
  const { t } = useTranslation()
  const loopConfig = data.loopConfig
  if (loopConfig === undefined) {
    return null
  }
  const update = (outputs: WorkflowOutputBinding[]): void => {
    onChange({ ...data, loopConfig: { ...loopConfig, outputs } })
  }
  return (
    <InspectorField label={t('workflows.loop.field.outputs')} htmlFor="workflow-loop-outputs">
      <div className="space-y-2" id="workflow-loop-outputs">
        {/* oxlint-disable react/no-array-index-key -- a named output has no stable id; its name is the editable identity, so a content key collides on a blank new row. */}
        {loopConfig.outputs.map((output, index) => (
          // oxlint-disable-next-line react/no-array-index-key -- a named output has no stable id; its name is the editable identity, so a content key collides on a blank new row.
          <div key={`loop-output-${index}`} className="flex items-center gap-2">
            <Input
              aria-label={`${t('workflows.loop.field.outputName')} ${index + 1}`}
              value={output.name}
              onChange={(event) =>
                update(
                  loopConfig.outputs.map((current, i) =>
                    i === index ? { ...current, name: event.target.value } : current,
                  ),
                )
              }
            />
            <Input
              aria-label={`${t('workflows.loop.field.outputSelector')} ${index + 1}`}
              placeholder={t('workflows.loop.selectorPlaceholder')}
              value={selectorToText(output.variableSelector)}
              onChange={(event) =>
                update(
                  loopConfig.outputs.map((current, i) =>
                    i === index
                      ? { ...current, variableSelector: textToSelector(event.target.value) }
                      : current,
                  ),
                )
              }
            />
            <Button
              type="button"
              variant="ghost"
              size="icon"
              aria-label={`${t('workflows.loop.removeOutput')} ${index + 1}`}
              onClick={() => update(loopConfig.outputs.filter((_, i) => i !== index))}
            >
              <Trash2 />
            </Button>
          </div>
        ))}
      </div>
      <Button
        type="button"
        variant="outline"
        size="sm"
        onClick={() => update([...loopConfig.outputs, { name: '', variableSelector: [] }])}
      >
        <Plus />
        {t('workflows.loop.addOutput')}
      </Button>
    </InspectorField>
  )
}

/** Explains the loopExit marker: it ends the round and carries no configuration of its own. */
export function LoopExitNodeFields() {
  const { t } = useTranslation()
  return (
    <p className="rounded-lg border border-border bg-muted/25 px-3 py-2 text-[11px] leading-5 text-muted-foreground">
      {t('workflows.node.loopExit.description')}
    </p>
  )
}

/** Notes that a Subflow node is a placeholder until the V2 execution engine lands. */
export function SubflowNodeFields() {
  const { t } = useTranslation()
  return (
    <p className="rounded-lg border border-border bg-muted/25 px-3 py-2 text-[11px] leading-5 text-muted-foreground">
      {t('workflows.subflow.hint')}
    </p>
  )
}

/** A stable per-row key for one tool call parameter, built from its content. */
function toolParameterKey(parameter: WorkflowToolParameter): string {
  return `${parameter.key}::${parameter.value}`
}
