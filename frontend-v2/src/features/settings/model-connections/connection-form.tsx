import { useId, useState } from 'react'
import { Plus, Trash2 } from 'lucide-react'
import type { ModelConnection } from '@/api/generated.schemas'
import { Button } from '@/components/ui/button'
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
  FieldLegend,
  FieldSet,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Switch } from '@/components/ui/switch'
import { ActionError } from '../components/page-parts'
import { validateConnection } from '../data/resource-actions'
import type { ConnectionDraft } from '../data/types'

export function ConnectionForm({
  connection,
  pending,
  error,
  onSave,
  onCancel,
}: {
  connection?: ModelConnection
  pending: boolean
  error: unknown
  onSave: (draft: ConnectionDraft, expectedVersion?: number) => void
  onCancel: () => void
}) {
  const id = useId()
  const [expectedVersion] = useState(connection?.version)
  const [draft, setDraft] = useState<ConnectionDraft>(() => ({
    name: connection?.name ?? '',
    protocol: connection?.protocol ?? 'openai-completions',
    authMode: connection?.authMode ?? 'bearer',
    baseUrl: connection?.baseUrl ?? '',
    enabled: connection?.enabled ?? true,
    models: connection
      ? structuredClone(connection.models)
      : [{ id: '', name: '', contextWindow: 128000, maxTokens: 8192 }],
  }))
  const [validation, setValidation] = useState('')
  const updateModel = (
    index: number,
    field: 'id' | 'name' | 'contextWindow' | 'maxTokens',
    value: string | number,
  ) =>
    setDraft((current) => ({
      ...current,
      models: current.models.map((model, i) =>
        i === index ? { ...model, [field]: value } : model,
      ),
    }))
  return (
    <form
      className="flex flex-col gap-6"
      onSubmit={(event) => {
        event.preventDefault()
        const message = validateConnection(draft)
        setValidation(message ?? '')
        if (!message) onSave(draft, expectedVersion)
      }}
    >
      <FieldGroup>
        <Field>
          <FieldLabel htmlFor={id + '-name'}>连接名称</FieldLabel>
          <Input
            id={id + '-name'}
            required
            maxLength={120}
            value={draft.name}
            onChange={(e) => setDraft({ ...draft, name: e.target.value })}
            disabled={pending}
            placeholder="例如 日常写作模型"
          />
        </Field>
        <Field>
          <FieldLabel htmlFor={id + '-protocol'}>协议</FieldLabel>
          <NativeSelect
            id={id + '-protocol'}
            value={draft.protocol}
            disabled={pending}
            onChange={(e) => {
              const protocol = e.target.value
              if (
                protocol === 'openai-completions' ||
                protocol === 'anthropic-messages'
              )
                setDraft({ ...draft, protocol, authMode: 'bearer' })
            }}
          >
            <NativeSelectOption value="openai-completions">
              OpenAI Chat Completions
            </NativeSelectOption>
            <NativeSelectOption value="anthropic-messages">
              Anthropic Messages
            </NativeSelectOption>
          </NativeSelect>
        </Field>
        <Field>
          <FieldLabel htmlFor={id + '-url'}>服务地址</FieldLabel>
          <Input
            id={id + '-url'}
            required
            type="url"
            value={draft.baseUrl}
            placeholder="https://models.example.com/v1"
            disabled={pending}
            onChange={(e) => setDraft({ ...draft, baseUrl: e.target.value })}
          />
          <FieldDescription>
            仅保存演示元数据，不会访问此地址。
          </FieldDescription>
        </Field>
        <Field>
          <FieldLabel htmlFor={id + '-auth'}>认证方式</FieldLabel>
          <NativeSelect
            id={id + '-auth'}
            value={draft.authMode}
            disabled={pending}
            onChange={(e) => {
              const authMode = e.target.value
              if (authMode === 'bearer' || authMode === 'x-api-key')
                setDraft({ ...draft, authMode })
            }}
          >
            <NativeSelectOption value="bearer">Bearer</NativeSelectOption>
            {draft.protocol === 'anthropic-messages' && (
              <NativeSelectOption value="x-api-key">
                x-api-key
              </NativeSelectOption>
            )}
          </NativeSelect>
        </Field>
        <Field orientation="horizontal">
          <FieldLabel htmlFor={id + '-enabled'}>启用演示连接</FieldLabel>
          <Switch
            id={id + '-enabled'}
            checked={draft.enabled}
            disabled={pending}
            onCheckedChange={(enabled) => setDraft({ ...draft, enabled })}
          />
        </Field>
      </FieldGroup>
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-medium">模型</h3>
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={pending || draft.models.length >= 100}
          onClick={() =>
            setDraft({
              ...draft,
              models: [
                ...draft.models,
                { id: '', name: '', contextWindow: 128000, maxTokens: 8192 },
              ],
            })
          }
        >
          <Plus data-icon="inline-start" />
          添加模型
        </Button>
      </div>
      {draft.models.map((model, index) => (
        <FieldSet key={index} className="rounded-lg border p-4">
          <FieldLegend>模型 {index + 1}</FieldLegend>
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor={id + '-model-' + index}>模型标识</FieldLabel>
              <Input
                id={id + '-model-' + index}
                required
                value={model.id}
                disabled={pending}
                onChange={(e) => updateModel(index, 'id', e.target.value)}
                placeholder="provider/model-name"
              />
            </Field>
            <Field>
              <FieldLabel htmlFor={id + '-label-' + index}>显示名称</FieldLabel>
              <Input
                id={id + '-label-' + index}
                required
                value={model.name}
                disabled={pending}
                onChange={(e) => updateModel(index, 'name', e.target.value)}
              />
            </Field>
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <Field>
                <FieldLabel htmlFor={id + '-context-' + index}>
                  上下文上限
                </FieldLabel>
                <Input
                  id={id + '-context-' + index}
                  required
                  type="number"
                  min={1}
                  step={1}
                  value={model.contextWindow}
                  disabled={pending}
                  onChange={(e) =>
                    updateModel(index, 'contextWindow', Number(e.target.value))
                  }
                />
              </Field>
              <Field>
                <FieldLabel htmlFor={id + '-tokens-' + index}>
                  输出上限
                </FieldLabel>
                <Input
                  id={id + '-tokens-' + index}
                  required
                  type="number"
                  min={1}
                  step={1}
                  value={model.maxTokens}
                  disabled={pending}
                  onChange={(e) =>
                    updateModel(index, 'maxTokens', Number(e.target.value))
                  }
                />
              </Field>
            </div>
          </FieldGroup>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            disabled={pending || draft.models.length === 1}
            onClick={() =>
              setDraft({
                ...draft,
                models: draft.models.filter((_, i) => i !== index),
              })
            }
          >
            <Trash2 data-icon="inline-start" />
            移除模型 {index + 1}
          </Button>
        </FieldSet>
      ))}
      <ActionError error={validation ? new Error(validation) : error} />
      <div className="settings-form-actions">
        <Button
          type="button"
          variant="outline"
          disabled={pending}
          onClick={onCancel}
        >
          取消
        </Button>
        <Button type="submit" disabled={pending}>
          {pending ? '保存中…' : '保存连接'}
        </Button>
      </div>
    </form>
  )
}
