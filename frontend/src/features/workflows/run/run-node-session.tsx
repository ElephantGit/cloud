import { useTranslation } from 'react-i18next'
import type { WorkflowNodeSessionExecutionState } from '@/api/generated.schemas'
import { Button } from '@/components/ui/button'
import { useWorkflowNodeSession } from '@/features/workflows/api'
import { InspectorSectionTitle } from '@/features/workflows/run/run-inspector'
import {
  nodeSessionView,
  type NodeSessionRow,
  type NodeSessionView,
} from '@/features/workflows/run/run-node-session-view'

/**
 * Translation keys for the read states that carry no records. `available` is absent because that
 * state renders the records themselves, so there is no message for it to fall back to.
 */
const STATE_MESSAGES = {
  not_applicable: 'workflows.run.session.notApplicable',
  not_started: 'workflows.run.session.notStarted',
  awaiting_first_record: 'workflows.run.session.awaitingFirstRecord',
  unavailable: 'workflows.run.session.unavailable',
  unknown: 'workflows.run.session.unknown',
} as const satisfies Record<Exclude<WorkflowNodeSessionExecutionState, 'available'>, string>

/** Identifies the run and its graph node whose recorded conversation the inspector shows. */
export interface RunNodeSessionProps {
  tenantId: string
  workflowId: string
  runId: string
  nodeId: string
}

/**
 * The inspector's conversation section for one agent node attempt.
 *
 * It shows what the run's receipts prove and nothing else: a state Cloud cannot resolve is named as
 * such instead of being drawn as an empty conversation, the end reason stands on its own line
 * rather than implying the output, and a page that observed missing receipts says so.
 *
 * @param props - The tenant, workflow, run and node the read is scoped to.
 */
export function RunNodeSession({ tenantId, workflowId, runId, nodeId }: RunNodeSessionProps) {
  const { t } = useTranslation()
  const session = useWorkflowNodeSession({ tenantId, workflowId, runId, nodeId })
  const view = session.data === undefined ? undefined : nodeSessionView(session.data.pages)
  return (
    <section aria-label={t('workflows.run.session.title')}>
      <InspectorSectionTitle>{t('workflows.run.session.title')}</InspectorSectionTitle>
      {view === undefined ? (
        <SessionFallback pending={session.isPending} />
      ) : (
        <SessionBody
          view={view}
          fetchingMore={session.isFetchingNextPage}
          onLoadMore={() => void session.fetchNextPage()}
        />
      )}
    </section>
  )
}

/** The read's own failure, which is not the node having recorded nothing. */
function SessionFallback({ pending }: { pending: boolean }) {
  const { t } = useTranslation()
  return (
    <p className="rounded-md border border-dashed border-border px-3 py-3 text-[11px] text-muted-foreground">
      {pending ? t('workflows.run.session.loading') : t('workflows.run.session.failed')}
    </p>
  )
}

/** The attempt's identity, its end reason, and either its records or the reason it has none. */
function SessionBody({
  view,
  fetchingMore,
  onLoadMore,
}: {
  view: NodeSessionView
  fetchingMore: boolean
  onLoadMore: () => void
}) {
  const { t } = useTranslation()
  return (
    <div className="space-y-2">
      <p className="text-[11px] text-muted-foreground">
        {t('workflows.run.session.attempt', { attempt: view.attempt })}
      </p>
      {view.endReason !== null && (
        <div className="flex items-baseline justify-between gap-3 text-xs">
          <span className="shrink-0 text-muted-foreground">
            {t('workflows.run.session.endReason')}
          </span>
          <span className="m-0 text-right font-mono text-[11px] break-words">{view.endReason}</span>
        </div>
      )}
      {view.state === 'available' ? (
        <SessionRecords view={view} fetchingMore={fetchingMore} onLoadMore={onLoadMore} />
      ) : (
        <p className="rounded-md border border-dashed border-border px-3 py-3 text-[11px] text-muted-foreground">
          {t(STATE_MESSAGES[view.state])}
        </p>
      )}
    </div>
  )
}

/** The loaded records oldest first, the integrity warning, and the walk's earlier pages. */
function SessionRecords({
  view,
  fetchingMore,
  onLoadMore,
}: {
  view: NodeSessionView
  fetchingMore: boolean
  onLoadMore: () => void
}) {
  const { t } = useTranslation()
  return (
    <div className="space-y-2">
      {view.incomplete && (
        <p className="rounded-md border border-border bg-muted/60 px-2 py-1.5 text-[11px] text-muted-foreground">
          {t('workflows.run.session.incomplete')}
        </p>
      )}
      {view.hasMore && (
        <Button
          size="sm"
          variant="outline"
          onClick={onLoadMore}
          disabled={fetchingMore}
          className="w-full"
        >
          {t('workflows.run.session.loadEarlier')}
        </Button>
      )}
      {view.rows.length === 0 ? (
        <p className="rounded-md border border-dashed border-border px-3 py-3 text-[11px] text-muted-foreground">
          {t('workflows.run.session.empty')}
        </p>
      ) : (
        <ol aria-label={t('workflows.run.session.records')} className="space-y-2">
          {view.rows.map((row) => (
            <li key={row.key}>
              <SessionRow row={row} />
            </li>
          ))}
        </ol>
      )}
      <p className="text-[10px] text-muted-foreground">
        {t('workflows.run.session.loaded', { count: view.rows.length })}
      </p>
    </div>
  )
}

/** One record, styled by kind; a truncated record says it is only part of what was recorded. */
function SessionRow({ row }: { row: NodeSessionRow }) {
  const { t } = useTranslation()
  const truncated = row.truncated ? (
    <span className="ml-1 text-[10px] text-muted-foreground">
      {t('workflows.run.session.truncated')}
    </span>
  ) : null
  if (row.kind === 'user') {
    return (
      <p className="rounded border-l-2 border-primary/60 bg-muted/40 px-2 py-1 text-xs leading-5 break-words whitespace-pre-wrap">
        <span className="mr-1 text-[10px] text-muted-foreground">
          {t('workflows.run.session.userInput')}
        </span>
        {row.text}
        {truncated}
      </p>
    )
  }
  if (row.kind === 'message') {
    return (
      <p className="text-xs leading-5 break-words whitespace-pre-wrap">
        {row.text}
        {truncated}
      </p>
    )
  }
  if (row.kind === 'thought') {
    return (
      <p className="border-l-2 border-border pl-2 text-[11px] leading-5 text-muted-foreground italic break-words whitespace-pre-wrap">
        {row.text}
        {truncated}
      </p>
    )
  }
  if (row.kind === 'tool') {
    return (
      <p className="rounded bg-muted/60 px-2 py-1 font-mono text-[11px] break-words">
        {row.text}
        {truncated}
      </p>
    )
  }
  if (row.kind === 'turnEnded') {
    return (
      <p className="text-[10px] text-muted-foreground">
        {t('workflows.run.session.turnEnded', { reason: row.text === '' ? '—' : row.text })}
      </p>
    )
  }
  if (row.kind === 'gap') {
    return (
      <p className="rounded border border-dashed border-border px-2 py-1 text-[10px] text-muted-foreground">
        {t('workflows.run.session.gap')}
      </p>
    )
  }
  return (
    <pre className="max-h-40 overflow-auto rounded bg-muted/40 px-2 py-1 font-mono text-[10px] break-words whitespace-pre-wrap">
      {row.text}
      {truncated}
    </pre>
  )
}
