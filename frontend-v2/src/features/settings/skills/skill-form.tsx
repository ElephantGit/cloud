import { useId, useState } from 'react'
import { Button } from '@/components/ui/button'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { ActionError } from '../components/page-parts'
import type { Skill, SkillInput } from '@/lib/skills-api/generated'

export function SkillForm({
  skill,
  pending,
  error,
  onSave,
  onCancel,
}: {
  skill?: Skill
  pending: boolean
  error: unknown
  onSave: (draft: SkillInput) => void
  onCancel: () => void
}) {
  const id = useId()
  const [draft, setDraft] = useState<SkillInput>({
    name: skill?.name ?? '',
    description: skill?.description ?? '',
    content: skill?.content ?? '',
  })
  return (
    <form
      className="flex flex-col gap-6"
      onSubmit={(event) => {
        event.preventDefault()
        onSave(draft)
      }}
    >
      <FieldGroup>
        <Field>
          <FieldLabel htmlFor={id + '-name'}>技能名称</FieldLabel>
          <Input
            id={id + '-name'}
            required
            maxLength={255}
            value={draft.name}
            onChange={(e) => setDraft({ ...draft, name: e.target.value })}
            placeholder="例如 change-summary"
            disabled={pending}
          />
        </Field>
        <Field>
          <FieldLabel htmlFor={id + '-description'}>简介</FieldLabel>
          <Textarea
            id={id + '-description'}
            required
            maxLength={4096}
            value={draft.description}
            onChange={(e) =>
              setDraft({ ...draft, description: e.target.value })
            }
            placeholder="说明这个技能适合什么任务"
            disabled={pending}
          />
        </Field>
        <Field>
          <FieldLabel htmlFor={id + '-content'}>
            说明文档（Markdown）
          </FieldLabel>
          <Textarea
            className="min-h-64 font-mono text-sm"
            id={id + '-content'}
            maxLength={921600}
            value={draft.content}
            onChange={(e) => setDraft({ ...draft, content: e.target.value })}
            placeholder="# 使用说明"
            disabled={pending}
          />
        </Field>
      </FieldGroup>
      <ActionError error={error} />
      <p className="text-xs leading-relaxed text-muted-foreground">
        保存后会写入服务端技能目录，新开启的 Agent 会话会自动发现它。
      </p>
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
          {pending ? '保存中…' : '保存技能'}
        </Button>
      </div>
    </form>
  )
}
