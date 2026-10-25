import { useTranslation } from 'react-i18next'
import { Plus, Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { InspectorField } from '@/features/workflows/editor/workflow-inspector-fields'
import {
  selectorToText,
  textToSelector,
} from '@/features/workflows/editor/workflow-variable-select'
import type { WorkflowNodeData } from '@/features/workflows/runtime/types'

/** Props every Aggregator panel component takes: the node data and the writer for the whole node. */
export interface AggregatorFieldsProps {
  data: WorkflowNodeData
  onChange: (data: WorkflowNodeData) => void
}

/**
 * Edits the aggregator's ordered candidates.
 *
 * Order is the contract: the first assigned candidate is the one the node publishes, so the panel
 * never reorders the list on its own and exposes the position as a plain number. A selector is
 * `node.root` text until the shared picker replaces it; the publish gate refuses an undeclared or
 * nested selector, so a typo is caught before the graph runs.
 */
export function AggregatorNodeFields({ data, onChange }: AggregatorFieldsProps) {
  const { t } = useTranslation()
  const config = data.aggregatorConfig
  if (config === undefined) {
    return null
  }
  const update = (variables: string[][]): void => {
    onChange({ ...data, aggregatorConfig: { variables } })
  }
  return (
    <InspectorField
      label={t('workflows.aggregator.field.variables')}
      htmlFor="workflow-aggregator-variables"
    >
      <div className="space-y-2" id="workflow-aggregator-variables">
        {/* oxlint-disable react/no-array-index-key -- a candidate has no stable identity: its selector is the editable value, so a content key collides on two blank rows. */}
        {config.variables.map((selector, index) => (
          <div key={`aggregator-candidate-${index}`} className="flex items-center gap-2">
            <span className="w-4 shrink-0 text-[11px] text-muted-foreground">{index + 1}</span>
            <Input
              aria-label={`${t('workflows.aggregator.field.candidate')} ${index + 1}`}
              placeholder={t('workflows.aggregator.selectorPlaceholder')}
              value={selectorToText(selector)}
              onChange={(event) =>
                update(
                  config.variables.map((current, i) =>
                    i === index ? textToSelector(event.target.value) : current,
                  ),
                )
              }
            />
            <Button
              type="button"
              variant="ghost"
              size="icon"
              aria-label={`${t('workflows.aggregator.removeCandidate')} ${index + 1}`}
              onClick={() => update(config.variables.filter((_, i) => i !== index))}
            >
              <Trash2 />
            </Button>
          </div>
        ))}
        {/* oxlint-enable react/no-array-index-key */}
      </div>
      <Button
        type="button"
        variant="outline"
        size="sm"
        onClick={() => update([...config.variables, []])}
      >
        <Plus />
        {t('workflows.aggregator.addCandidate')}
      </Button>
    </InspectorField>
  )
}
