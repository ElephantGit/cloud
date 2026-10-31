import { useState, type FormEvent, type ReactNode } from 'react'
import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { validateSquad } from './squad-model'

/** The editable squad text; the server rejects a name that still has outer whitespace. */
export interface SquadDraft {
  name: string
  description: string
  charter: string
}

/**
 * Create or edit the squad's own fields. The leader is not edited here: it is set membership, so
 * it stays with the member operations.
 */
export function SquadForm({ initial, submitLabel, saving, error, onSave, onCancel }: {
  initial?: SquadDraft
  submitLabel: string
  saving: boolean
  /** Error from the last save attempt, shown above the actions. */
  error: string
  onSave: (draft: SquadDraft) => void
  onCancel: () => void
}) {
  const [draft, setDraft] = useState<SquadDraft>(initial ?? { name: '', description: '', charter: '' })
  const [errors, setErrors] = useState<Record<string, string>>({})
  const patch = (changes: Partial<SquadDraft>) => {
    setDraft(current => ({ ...current, ...changes }))
    setErrors({})
  }
  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const found = validateSquad(draft.name, draft.description, draft.charter)
    setErrors(found)
    if (Object.keys(found).length) return
    onSave({ name: draft.name.trim(), description: draft.description, charter: draft.charter })
  }
  return <form className="mc-form" onSubmit={submit} noValidate>
    <div className="mc-form-scroll"><FieldGroup>
      <SquadField name="name" label="小队名称" error={errors.name} hint="名称会显示在列表与小队详情里。">
        <Input id="mc-name" placeholder="例如：产品增长小队" value={draft.name} onChange={e => patch({ name: e.target.value })} aria-invalid={!!errors.name} aria-describedby={errors.name ? 'mc-error-name' : undefined} />
      </SquadField>
      <SquadField name="description" label="简介" error={errors.description} hint="一句话说明这个小队承接什么。">
        <Textarea id="mc-description" rows={2} placeholder="例如：负责新用户首次体验的调研与改进。" value={draft.description} onChange={e => patch({ description: e.target.value })} aria-invalid={!!errors.description} aria-describedby={errors.description ? 'mc-error-description' : undefined} />
      </SquadField>
      <SquadField name="charter" label="协作说明" error={errors.charter} hint="约定人和 Agent 的分工与节奏；不触发自动协作。">
        <Textarea id="mc-charter" rows={4} placeholder="例如：人类定优先级，Agent 收集数据；每天上午同步一次。" value={draft.charter} onChange={e => patch({ charter: e.target.value })} aria-invalid={!!errors.charter} aria-describedby={errors.charter ? 'mc-error-charter' : undefined} />
      </SquadField>
    </FieldGroup></div>
    {error && <p role="alert" className="mc-form-error">{error}</p>}
    <footer className="mc-form-actions">
      <Button type="button" variant="outline" onClick={onCancel} disabled={saving}>取消</Button>
      <Button type="submit" disabled={saving}>{saving ? '保存中…' : submitLabel}</Button>
    </footer>
  </form>
}

function SquadField({ name, label, error, hint, children }: { name: string; label: string; error?: string; hint: string; children: ReactNode }) {
  return <Field data-invalid={!!error}>
    <FieldLabel htmlFor={`mc-${name}`}>{label}</FieldLabel>
    {children}
    <FieldDescription>{hint}</FieldDescription>
    <FieldError id={`mc-error-${name}`}>{error}</FieldError>
  </Field>
}
