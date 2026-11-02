import { useId, useState } from 'react'
import { Button } from '@/components/ui/button'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { ActionError } from '../components/page-parts'
import type { Role, RoleDraft } from '../data/types'

export function RoleForm({
  role,
  pending,
  error,
  onSave,
  onCancel,
}: {
  role?: Role
  pending: boolean
  error: unknown
  onSave: (draft: RoleDraft) => void
  onCancel: () => void
}) {
  const id = useId()
  const [draft, setDraft] = useState<RoleDraft>({
    name: role?.name ?? '',
    description: role?.description ?? '',
    content: role?.content ?? '',
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
          <FieldLabel htmlFor={id + '-name'}>标题</FieldLabel>
          <Input
            id={id + '-name'}
            required
            maxLength={80}
            value={draft.name}
            onChange={(event) =>
              setDraft({ ...draft, name: event.target.value })
            }
            placeholder="例如 代码审查"
            disabled={pending}
          />
        </Field>
        <Field>
          <FieldLabel htmlFor={id + '-description'}>描述</FieldLabel>
          <Textarea
            id={id + '-description'}
            maxLength={500}
            value={draft.description}
            onChange={(event) =>
              setDraft({ ...draft, description: event.target.value })
            }
            placeholder="一句话说明这个角色适合什么任务"
            disabled={pending}
          />
        </Field>
        <Field>
          <FieldLabel htmlFor={id + '-content'}>内容（Markdown）</FieldLabel>
          <Textarea
            className="min-h-64 font-mono text-sm"
            id={id + '-content'}
            maxLength={100000}
            value={draft.content}
            onChange={(event) =>
              setDraft({ ...draft, content: event.target.value })
            }
            placeholder="# 角色定义"
            disabled={pending}
          />
        </Field>
      </FieldGroup>
      <ActionError error={error} />
      <p className="text-xs leading-relaxed text-muted-foreground">
        角色只保存在当前浏览器。工作流节点可以选择它，但 v2 执行仍不会
        注入角色内容。
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
          {pending ? '保存中…' : '保存角色'}
        </Button>
      </div>
    </form>
  )
}
