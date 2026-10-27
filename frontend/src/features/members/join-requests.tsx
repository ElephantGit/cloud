import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { useDecideJoinRequest, type useJoinRequests } from '@/features/members/api'
import { entryDate } from '@/features/members/browser'

/** Shows actionable requests separately from decisions, optionally scoped to a share link. */
export function JoinRequests({
  tenantId,
  requests,
  linkFilter,
}: {
  tenantId: string
  requests: ReturnType<typeof useJoinRequests>
  linkFilter: string | undefined
}) {
  const { t, i18n } = useTranslation()
  const [processed, setProcessed] = useState(false)
  const decide = useDecideJoinRequest(tenantId)
  const rows = requests.data?.items
    .filter(
      (row) =>
        (!linkFilter || row.linkId === linkFilter) &&
        (processed ? row.status !== 'pending' : row.status === 'pending'),
    )
    .toSorted((a, b) => b.createdAt.localeCompare(a.createdAt))
  return (
    <section className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 className="font-medium">{t(processed ? 'entry.processed' : 'entry.requests')}</h2>
        <Button variant="outline" aria-pressed={processed} onClick={() => setProcessed(!processed)}>
          {t(processed ? 'entry.requests' : 'entry.processed')}
        </Button>
      </div>
      {linkFilter && (
        <p className="text-sm text-muted-foreground">
          {t('entry.source')} · #{linkFilter.slice(0, 8)}
        </p>
      )}
      {requests.isPending && <p role="status">{t('entry.loading')}</p>}
      {requests.isError && (
        <p role="alert">
          {t('entry.loadFailed')}{' '}
          <Button onClick={() => void requests.refetch()}>{t('entry.retry')}</Button>
        </p>
      )}
      {rows?.length === 0 && (
        <p className="rounded-lg border border-dashed p-8 text-center text-sm text-muted-foreground">
          {t(processed ? 'entry.emptyHistory' : 'entry.emptyRequests')}
        </p>
      )}
      {rows?.map((row) => (
        <article
          key={row.id}
          className="flex flex-wrap justify-between gap-4 rounded-lg border p-4 text-sm"
        >
          <div className="space-y-2">
            <p className="font-medium">
              {row.displayName || row.userId}{' '}
              <span className="ml-2 rounded bg-muted px-2 py-0.5 text-xs font-normal">
                {t(`entry.${row.status}`)}
              </span>
            </p>
            <p className="text-muted-foreground">
              {t('entry.appliedAt')} · {entryDate(row.createdAt, i18n.language)}
            </p>
            <p className="text-muted-foreground">
              {t('entry.source')} · {row.linkNote || `#${row.linkId.slice(0, 8)}`}
            </p>
            {row.decidedAt && (
              <p>
                {t('entry.decidedBy')} · {row.decidedByName || row.decidedBy} ·{' '}
                {entryDate(row.decidedAt, i18n.language)}
              </p>
            )}
          </div>
          {!processed && (
            <div className="flex gap-2">
              {(['approve', 'reject'] as const).map((decision) => (
                <Button
                  key={decision}
                  size="sm"
                  variant={decision === 'approve' ? 'default' : 'outline'}
                  disabled={decide.isPending}
                  onClick={() => decide.mutate({ id: row.id, version: row.version, decision })}
                >
                  {t(`entry.${decision}`)}
                </Button>
              ))}
            </div>
          )}
        </article>
      ))}
      {decide.isError && (
        <p role="alert" className="text-destructive">
          {t('entry.actionFailed')}
        </p>
      )}
    </section>
  )
}
