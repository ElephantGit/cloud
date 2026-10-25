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
import {
  createDefaultWorkflowCapabilities,
  type WorkflowChoice,
} from '@/features/workflows/runtime/capabilities'
import type {
  WorkflowConditionComparison,
  WorkflowLoopConfig,
  WorkflowLoopVariable,
  WorkflowNodeData,
  WorkflowOutputBinding,
} from '@/features/workflows/runtime/types'
import type { TranslationKey } from '@/i18n/i18n-instance'
import { InspectorField } from '@/features/workflows/editor/workflow-inspector-fields'
import {
  LocalizedChoiceSelect,
  WorkflowChoiceSelect,
} from '@/features/workflows/editor/workflow-choice-select'
import {
  WorkflowVariableTypeMenu,
  comparisonValueToText,
  parseComparisonValue,
  selectorToText,
  textToSelector,
} from '@/features/workflows/editor/workflow-variable-select'

/** Props every Loop panel component takes: the node data and the writer for the whole node. */
interface LoopFieldsProps {
  data: WorkflowNodeData
  onChange: (data: WorkflowNodeData) => void
}
/** Exhaustion policies of a Loop, keyed to the graph's enum names. */
const WORKFLOW_LOOP_BEHAVIOR_CHOICES = [
  { value: 'fail', labelKey: 'workflows.loop.behavior.fail' },
  { value: 'succeed', labelKey: 'workflows.loop.behavior.succeed' },
] as const satisfies readonly WorkflowChoice[]

/** Edits a Loop container: budget, carried initial value, and the behavior note. */
export function LoopNodeFields({ data, onChange }: LoopFieldsProps) {
  const { t } = useTranslation()
  const loopConfig = data.loopConfig
  return (
    <>
      <LoopBudgetFields data={data} onChange={onChange} />
      <LoopVariablesFields data={data} onChange={onChange} />
      <LoopUntilFields data={data} onChange={onChange} />
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
function LoopBudgetFields({ data, onChange }: LoopFieldsProps) {
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
function LoopVariablesFields({ data, onChange }: LoopFieldsProps) {
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

/** How a Loop's round-end rules combine. */
const LOOP_UNTIL_LOGIC_CHOICES = [
  { value: 'and', labelKey: 'workflows.condition.logicAnd' },
  { value: 'or', labelKey: 'workflows.condition.logicOr' },
] as const satisfies readonly WorkflowChoice[]

/** Operators that answer a question about the value, so they take no expected value. */
const LOOP_OPERATORS_WITHOUT_VALUE = new Set(['exists', 'not_exists', 'empty', 'not_empty'])

/** Edits the round-end condition: how rules combine and the rules themselves. */
function LoopUntilFields({ data, onChange }: LoopFieldsProps) {
  const { t } = useTranslation()
  const loopConfig = data.loopConfig
  if (loopConfig === undefined) {
    return null
  }
  const until = loopConfig.until
  const update = (next: WorkflowLoopConfig['until']): void => {
    onChange({ ...data, loopConfig: { ...loopConfig, until: next } })
  }
  return (
    <InspectorField label={t('workflows.loop.field.until')} htmlFor="workflow-loop-until">
      <div className="space-y-2" id="workflow-loop-until">
        <WorkflowChoiceSelect
          id="workflow-loop-until-logic"
          ariaLabel={t('workflows.loop.field.until')}
          value={until.logic}
          options={LOOP_UNTIL_LOGIC_CHOICES}
          className="h-8 w-full"
          onChange={(logic) => {
            if (logic === 'and' || logic === 'or') {
              update({ ...until, logic })
            }
          }}
        />
        {/* oxlint-disable react/no-array-index-key -- a rule has no stable id; its selector is the editable identity, so a content key collides on a blank new rule. */}
        {until.conditions.map((condition, index) => (
          <LoopUntilRow
            key={`loop-until-${index}`}
            index={index}
            condition={condition}
            onChange={(next) =>
              update({
                ...until,
                conditions: until.conditions.map((current, i) => (i === index ? next : current)),
              })
            }
            onRemove={() =>
              update({
                ...until,
                conditions: until.conditions.filter((_, i) => i !== index),
              })
            }
          />
        ))}
        {/* oxlint-enable react/no-array-index-key */}
      </div>
      <Button
        type="button"
        variant="outline"
        size="sm"
        onClick={() =>
          update({
            ...until,
            conditions: [...until.conditions, { variableSelector: [], operator: 'equals' }],
          })
        }
      >
        <Plus />
        {t('workflows.loop.addUntilRule')}
      </Button>
    </InspectorField>
  )
}

/** One round-end comparison: a variable, an operator and, for value operators, the expected value. */
function LoopUntilRow({
  index,
  condition,
  onChange,
  onRemove,
}: {
  index: number
  condition: WorkflowConditionComparison
  onChange: (condition: WorkflowConditionComparison) => void
  onRemove: () => void
}) {
  const { t } = useTranslation()
  const capabilities = createDefaultWorkflowCapabilities()
  return (
    <div className="space-y-2 rounded-lg border border-border p-2">
      <div className="flex items-center justify-between">
        <span className="text-[11px] font-medium text-muted-foreground">IF {index + 1}</span>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          aria-label={`${t('workflows.loop.removeUntilRule')} ${index + 1}`}
          onClick={onRemove}
        >
          <Trash2 />
        </Button>
      </div>
      <Input
        aria-label={t('workflows.condition.field.variable', { index: index + 1 })}
        placeholder={t('workflows.loop.selectorPlaceholder')}
        value={selectorToText(condition.variableSelector)}
        onChange={(event) =>
          onChange({ ...condition, variableSelector: textToSelector(event.target.value) })
        }
      />
      <WorkflowChoiceSelect
        id={`workflow-loop-until-operator-${index}`}
        ariaLabel={t('workflows.condition.field.operator', { index: index + 1 })}
        value={condition.operator}
        options={capabilities.conditionOperators}
        className="h-8 w-full"
        onChange={(operator) => {
          if (operator !== null) {
            onChange({ ...condition, operator })
          }
        }}
      />
      {LOOP_OPERATORS_WITHOUT_VALUE.has(condition.operator) ? null : (
        <Input
          aria-label={t('workflows.condition.field.value', { index: index + 1 })}
          value={comparisonValueToText(condition.value)}
          onChange={(event) =>
            onChange({ ...condition, value: parseComparisonValue(event.target.value) })
          }
        />
      )}
    </div>
  )
}

/** Edits the named results the loop publishes when it succeeds. */
function LoopOutputsFields({ data, onChange }: LoopFieldsProps) {
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
