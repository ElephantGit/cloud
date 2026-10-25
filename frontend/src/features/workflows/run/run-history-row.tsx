import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import type { WorkflowRun } from '@/api/generated.schemas'
import { RunStatusBadge } from '@/features/workflows/run/run-status-mark'
import { formatRunInstant, shortRunId } from '@/features/workflows/run/run-time'

/**
 * One run in the history: when it ran, how it ended, which version it executed, and the way in.
 *
 * The whole row is a link, so a member does not have to aim at a small control to open a run, and
 * the snapshot is shown as the frozen version label the rest of the surface uses — the run detail
 * page is that same frozen document, never the workflow's current draft.
 *
 * @param props.run - The run to render.
 * @param props.href - Destination of the row's link, the run's read-only detail page.
 */
export function RunHistoryRow({ run, href }: { run: WorkflowRun; href: string }) {
  const { t } = useTranslation()
  return (
    <li>
      <Link
        to={href}
        aria-label={t('workflows.runs.open', { name: run.name })}
        className="flex flex-wrap items-center gap-x-3 gap-y-1 rounded-lg border px-3 py-2 hover:bg-muted/50"
      >
        <RunStatusBadge status={run.status} className="shrink-0" />
        <span className="min-w-0 flex-1 truncate text-sm font-medium">{run.name}</span>
        <span className="shrink-0 text-xs text-muted-foreground">
          {formatRunInstant(run.startedAt ?? run.createdAt)}
        </span>
        <span className="shrink-0 rounded-md bg-muted px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground">
          {t('workflows.runs.version', { id: shortRunId(run.snapshotId) })}
        </span>
      </Link>
    </li>
  )
}
