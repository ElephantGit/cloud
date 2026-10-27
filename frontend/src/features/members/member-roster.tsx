import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogTitle, DialogDescription } from '@/components/ui/dialog'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
} from '@/components/ui/dropdown-menu'
import { useMemberJoinHistory, useUpdateMember, type TenantMember } from '@/features/members/api'
import { entryDate } from '@/features/members/browser'

/** Renders authoritative member state; provenance and actions are administrator-only. */
export function MemberRoster({
  tenantId,
  members,
  canManage,
}: {
  tenantId: string
  members: TenantMember[]
  canManage: boolean
}) {
  const { t, i18n } = useTranslation()
  const [selected, setSelected] = useState<TenantMember | undefined>()
  if (members.length === 0)
    return <p className="py-8 text-muted-foreground">{t('entry.emptyMembers')}</p>
  return (
    <>
      <div className="overflow-x-auto rounded-lg border">
        <table className="w-full text-left text-sm">
          <thead className="border-b bg-muted/40 text-muted-foreground">
            <tr>
              {[
                t('entry.name'),
                t('entry.role'),
                t('entry.status'),
                ...(canManage ? [t('entry.method'), t('entry.joinedAt'), ''] : []),
              ].map((label) => (
                <th key={label || 'actions'} className="whitespace-nowrap px-4 py-3 font-medium">
                  {label}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {members.map((member) => (
              <tr key={member.userId} className="border-b last:border-0">
                <td className="px-4 py-3 font-medium">{member.displayName || member.userId}</td>
                <td className="px-4 py-3">
                  {t(member.role === 'admin' ? 'entry.admin' : 'entry.member')}
                </td>
                <td className="whitespace-nowrap px-4 py-3 text-muted-foreground">
                  {t(member.status === 'active' ? 'entry.joined' : 'entry.disabled')}
                </td>
                {canManage && (
                  <>
                    <td className="px-4 py-3">
                      {t(`entry.method.${member.joinMethod ?? 'unknown'}`)}
                    </td>
                    <td className="whitespace-nowrap px-4 py-3 text-muted-foreground">
                      {entryDate(member.joinedAt, i18n.language)}
                    </td>
                    <td className="px-4 py-3">
                      <MemberActions
                        tenantId={tenantId}
                        member={member}
                        onDetails={() => setSelected(member)}
                      />
                    </td>
                  </>
                )}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {canManage && (
        <MemberDetails
          tenantId={tenantId}
          member={selected}
          onClose={() => setSelected(undefined)}
        />
      )}
    </>
  )
}

function MemberActions({
  tenantId,
  member,
  onDetails,
}: {
  tenantId: string
  member: TenantMember
  onDetails: () => void
}) {
  const { t } = useTranslation()
  const update = useUpdateMember(tenantId)
  const [confirm, setConfirm] = useState(false)
  const role = member.role === 'admin' ? 'admin' : 'member'
  return (
    <div className="flex items-center gap-2">
      <DropdownMenu>
        <DropdownMenuTrigger
          render={
            <Button variant="ghost" aria-label={`${t('entry.more')} · ${member.displayName}`} />
          }
        >
          •••
        </DropdownMenuTrigger>
        <DropdownMenuContent className="min-w-48 space-y-2 p-3">
          <DropdownMenuItem onClick={onDetails}>{t('entry.history')}</DropdownMenuItem>
          <Select
            value={role}
            disabled={member.status !== 'active' || update.isPending}
            onValueChange={(value) =>
              update.mutate({
                userId: member.userId,
                role: value === 'admin' ? 'admin' : 'member',
                status: 'active',
                version: member.version,
              })
            }
          >
            <SelectTrigger aria-label={`${member.displayName} ${t('entry.role')}`}>
              <SelectValue>{t(role === 'admin' ? 'entry.admin' : 'entry.member')}</SelectValue>
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="admin">{t('entry.admin')}</SelectItem>
              <SelectItem value="member">{t('entry.member')}</SelectItem>
            </SelectContent>
          </Select>
          {member.status === 'active' && (
            <DropdownMenuItem onClick={() => setConfirm(true)}>
              {t('entry.remove')}
            </DropdownMenuItem>
          )}
        </DropdownMenuContent>
      </DropdownMenu>
      {update.isError && (
        <p role="alert" className="text-xs text-destructive">
          {t('entry.actionFailed')}
        </p>
      )}
      <Dialog open={confirm} onOpenChange={setConfirm}>
        <DialogContent>
          <DialogTitle>
            {t('entry.remove')} · {member.displayName}
          </DialogTitle>
          <DialogDescription>{t('entry.removeHint')}</DialogDescription>
          <Button
            disabled={update.isPending}
            onClick={() =>
              update.mutate(
                { userId: member.userId, role, status: 'disabled', version: member.version },
                { onSuccess: () => setConfirm(false) },
              )
            }
          >
            {t('entry.remove')}
          </Button>
          <Button variant="outline" onClick={() => setConfirm(false)}>
            {t('entry.cancel')}
          </Button>
        </DialogContent>
      </Dialog>
    </div>
  )
}

function MemberDetails({
  tenantId,
  member,
  onClose,
}: {
  tenantId: string
  member: TenantMember | undefined
  onClose: () => void
}) {
  const { t, i18n } = useTranslation()
  const history = useMemberJoinHistory(tenantId, member?.userId)
  return (
    <Dialog
      open={!!member}
      onOpenChange={(open) => {
        if (!open) onClose()
      }}
    >
      <DialogContent className="max-h-[80vh] overflow-auto sm:max-w-lg">
        <DialogTitle>
          {t('entry.history')} · {member?.displayName}
        </DialogTitle>
        <DialogDescription>{t('entry.actor')}</DialogDescription>
        {history.isFetching && <p role="status">{t('entry.loading')}</p>}
        {history.isError && (
          <p role="alert">
            {t('entry.loadFailed')}{' '}
            <Button onClick={() => void history.refetch()}>{t('entry.retry')}</Button>
          </p>
        )}
        {history.data?.items.length === 0 && <p>{t('entry.noHistory')}</p>}
        {history.data?.items
          .toSorted((a, b) => b.joinedAt.localeCompare(a.joinedAt))
          .map((row) => (
            <div key={row.id} className="space-y-1 border-l-2 pl-3 text-sm">
              <p className="font-medium">{t(`entry.method.${row.method}`)}</p>
              <p>
                {row.actorDisplayName} · {entryDate(row.joinedAt, i18n.language)}
              </p>
              {(row.invitationId || row.requestId) && (
                <p className="break-all text-muted-foreground">
                  {row.invitationNote || row.linkNote || row.invitationId || row.requestId}
                </p>
              )}
            </div>
          ))}
      </DialogContent>
    </Dialog>
  )
}
