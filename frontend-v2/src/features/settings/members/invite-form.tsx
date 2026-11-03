import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { codeOf } from '@/lib/api-client'
import { listSquads } from '@/lib/squads-api/generated'
import { inviteMember } from '../api/generated'
import { membersKey } from './query'

const errors: Record<string, string> = {
  member_exists: '这个邮箱已经是团队成员。',
  user_unavailable: '这个账号已停用，无法邀请。',
  invalid_email: '邮箱格式不正确。',
  invalid_display_name: '显示名最多 64 个字。',
  forbidden: '只有 owner 可以邀请成员。',
  not_found: '工作空间不存在或已归档。',
}

/**
 * Owner-only invitation. The address alone creates the account, so the invitee signs in with it
 * and nothing else — there is no acceptance step. An optional squad joins the invitee to that
 * squad in the same transaction; the choices come from the workspace's live squads, so an
 * archived squad is never offered.
 */
export function InviteForm({ tenantId }: { tenantId: string }) {
  const client = useQueryClient()
  const [email, setEmail] = useState('')
  const [displayName, setDisplayName] = useState('')
  const [squadId, setSquadId] = useState('')
  const [invited, setInvited] = useState('')
  const squads = useQuery({ queryKey: ['squads', tenantId], queryFn: () => listSquads(tenantId), enabled: tenantId !== '' })
  const liveSquads = (squads.data?.items ?? []).filter((squad) => squad.archivedAt === null)
  const invite = useMutation({
    mutationFn: () =>
      inviteMember(tenantId, {
        email: email.trim(),
        displayName: displayName.trim() || undefined,
        squadId: squadId || undefined,
      }),
    onSuccess: async (member) => {
      setEmail('')
      setDisplayName('')
      setSquadId('')
      setInvited(member.displayName)
      await Promise.all([
        client.invalidateQueries({ queryKey: membersKey(tenantId) }),
        // The squad's version moved, so the 协作 page must not keep serving the old list.
        client.invalidateQueries({ queryKey: ['squads', tenantId] }),
      ])
    },
  })
  const message = errors[codeOf(invite.error) ?? ''] ?? '邀请失败，请重试。'

  return (
    <form
      aria-label="邀请成员"
      className="mb-6 flex flex-wrap items-end gap-3 rounded-lg border p-4"
      onSubmit={(event) => {
        event.preventDefault()
        setInvited('')
        invite.mutate()
      }}
    >
      <div className="flex min-w-48 flex-1 flex-col gap-2">
        <Label htmlFor="invite-email">邮箱</Label>
        <Input
          id="invite-email"
          type="email"
          required
          placeholder="teammate@example.com"
          value={email}
          onChange={(event) => setEmail(event.target.value)}
        />
      </div>
      <div className="flex min-w-40 flex-1 flex-col gap-2">
        <Label htmlFor="invite-name">显示名（选填）</Label>
        <Input
          id="invite-name"
          placeholder="默认取邮箱 @ 前缀"
          value={displayName}
          onChange={(event) => setDisplayName(event.target.value)}
        />
      </div>
      {liveSquads.length > 0 && (
        <div className="flex min-w-40 flex-1 flex-col gap-2">
          <Label htmlFor="invite-squad">小队（选填）</Label>
          <NativeSelect
            id="invite-squad"
            className="w-full"
            value={squadId}
            onChange={(event) => setSquadId(event.target.value)}
          >
            <NativeSelectOption value="">不加入小队</NativeSelectOption>
            {liveSquads.map((squad) => (
              <NativeSelectOption key={squad.id} value={squad.id}>
                {squad.name}
              </NativeSelectOption>
            ))}
          </NativeSelect>
        </div>
      )}
      <Button type="submit" disabled={invite.isPending || email.trim() === ''}>
        邀请
      </Button>
      {invited !== '' && (
        <p role="status" className="w-full text-sm text-muted-foreground">
          已邀请 {invited}，对方用这个邮箱登录即可。
        </p>
      )}
      {invite.isError && (
        <p role="alert" className="w-full text-sm text-destructive">
          {message}
        </p>
      )}
    </form>
  )
}
