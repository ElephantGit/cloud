import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { Invitation, JoinLink } from '@/api/generated.schemas'
import { Button } from '@/components/ui/button'
import {
  useInvitations,
  useJoinLinks,
  useRevokeInvitation,
  useRevokeJoinLink,
} from '@/features/members/api'
import { entryDate, linkState, useJoinClock } from '@/features/members/browser'
import { CreateLinkDialog } from '@/features/members/create-link-dialog'

/** Separates direct invitations from administrator-approved applications and their metadata. */
export function JoinLinks({
  tenantId,
  onViewRequests,
}: {
  tenantId: string
  onViewRequests: (id: string) => void
}) {
  const { t } = useTranslation()
  const invitations = useInvitations(tenantId)
  const applications = useJoinLinks(tenantId)
  const [creating, setCreating] = useState<'invite' | 'apply' | undefined>()
  return (
    <div className="space-y-6">
      <div className="grid gap-4 md:grid-cols-2">
        {(['invite', 'apply'] as const).map((kind) => (
          <section key={kind} className="space-y-3 rounded-lg border p-4">
            <h2 className="font-medium">{t(`entry.${kind}`)}</h2>
            <p className="min-h-10 text-sm text-muted-foreground">{t(`entry.${kind}Hint`)}</p>
            <Button
              variant={kind === 'invite' ? 'default' : 'outline'}
              onClick={() => setCreating(kind)}
            >
              {t('entry.generateTitle', { kind: t(`entry.${kind}`) })}
            </Button>
          </section>
        ))}
      </div>
      <LinkRecords
        kind="invite"
        query={invitations}
        tenantId={tenantId}
        onCreate={() => setCreating('invite')}
        onViewRequests={onViewRequests}
      />
      <LinkRecords
        kind="apply"
        query={applications}
        tenantId={tenantId}
        onCreate={() => setCreating('apply')}
        onViewRequests={onViewRequests}
      />
      {creating && (
        <CreateLinkDialog
          tenantId={tenantId}
          kind={creating}
          onClose={() => setCreating(undefined)}
        />
      )}
    </div>
  )
}

function LinkRecords({
  kind,
  query,
  tenantId,
  onCreate,
  onViewRequests,
}: {
  kind: 'invite' | 'apply'
  query: {
    data: { items: (Invitation | JoinLink)[] } | undefined
    isPending: boolean
    isError: boolean
    refetch: () => Promise<unknown>
  }
  tenantId: string
  onCreate: () => void
  onViewRequests: (id: string) => void
}) {
  const { t } = useTranslation()
  const [all, setAll] = useState(false)
  const now = useJoinClock()
  const rows = query.data?.items
    .filter((row) => all || linkState(row, now) === 'active')
    .toSorted((a, b) => b.createdAt.localeCompare(a.createdAt))
  return (
    <section className="space-y-3" aria-label={t(`entry.${kind}`)}>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 className="font-medium">{t(`entry.${kind}`)}</h2>
        <Button size="sm" variant="outline" aria-pressed={all} onClick={() => setAll(!all)}>
          {t(all ? 'entry.activeOnly' : 'entry.all')}
        </Button>
      </div>
      {query.isPending && <p role="status">{t('entry.loading')}</p>}
      {query.isError && (
        <p role="alert">
          {t('entry.loadFailed')}{' '}
          <Button onClick={() => void query.refetch()}>{t('entry.retry')}</Button>
        </p>
      )}
      {rows?.length === 0 && (
        <p className="rounded-lg border border-dashed p-5 text-sm text-muted-foreground">
          {t('entry.emptyLinks')}
        </p>
      )}
      {rows?.map((row) => (
        <LinkRecord
          key={row.id}
          row={row}
          kind={kind}
          tenantId={tenantId}
          state={linkState(row, now)}
          onCreate={onCreate}
          onViewRequests={onViewRequests}
        />
      ))}
    </section>
  )
}

function LinkRecord({
  row,
  kind,
  tenantId,
  state,
  onCreate,
  onViewRequests,
}: {
  row: Invitation | JoinLink
  kind: 'invite' | 'apply'
  tenantId: string
  state: ReturnType<typeof linkState>
  onCreate: () => void
  onViewRequests: (id: string) => void
}) {
  const { t, i18n } = useTranslation()
  const revokeInvitation = useRevokeInvitation(tenantId)
  const revokeLink = useRevokeJoinLink(tenantId)
  const revoke = kind === 'invite' ? revokeInvitation : revokeLink
  return (
    <article className="flex flex-wrap items-start justify-between gap-4 rounded-lg border p-4">
      <div className="min-w-0 space-y-2 text-sm">
        <div className="flex flex-wrap items-center gap-2">
          <h3 className="font-medium">
            {row.note || `${t(`entry.${kind}`)} · ${row.id.slice(0, 8)}`}
          </h3>
          <span className="rounded bg-muted px-2 py-0.5 text-xs">{t(`entry.${state}`)}</span>
        </div>
        <p className="text-muted-foreground">
          {t('entry.createdBy')} · {row.createdByName || row.createdBy} ·{' '}
          {entryDate(row.createdAt, i18n.language)}
        </p>
        <p className="text-muted-foreground">
          {t('entry.expiresAt')} · {entryDate(row.expiresAt, i18n.language)} · #{row.id.slice(0, 8)}
        </p>
        {'consumedAt' in row && row.consumedAt && (
          <p>
            {t('entry.usedBy', { name: row.consumedByName || row.consumedBy })} ·{' '}
            {entryDate(row.consumedAt, i18n.language)}
          </p>
        )}
        {kind === 'apply' && (
          <Button size="sm" variant="ghost" onClick={() => onViewRequests(row.id)}>
            {t('entry.applicationCounts', {
              pending: 'pendingCount' in row ? (row.pendingCount ?? 0) : 0,
              approved: 'approvedCount' in row ? (row.approvedCount ?? 0) : 0,
            })}{' '}
            · {t('entry.viewRequests')}
          </Button>
        )}
      </div>
      {state === 'active' ? (
        <Button
          size="sm"
          variant="outline"
          disabled={revoke.isPending}
          onClick={() => revoke.mutate({ id: row.id, version: row.version })}
        >
          {t('entry.revoke')}
        </Button>
      ) : (
        <Button size="sm" variant="outline" onClick={onCreate}>
          {t('entry.new')}
        </Button>
      )}
      {revoke.isError && (
        <p role="alert" className="w-full text-sm text-destructive">
          {t('entry.actionFailed')}
        </p>
      )}
    </article>
  )
}
