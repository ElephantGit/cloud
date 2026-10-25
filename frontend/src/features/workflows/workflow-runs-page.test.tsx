import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import type { WorkflowRun } from '@/api/generated.schemas'
import { WORKFLOW_RUN_PAGE_SIZE } from '@/features/workflows/api'
import { WorkflowRunsPage } from '@/features/workflows/workflow-runs-page'
import {
  installCloudSpaceHandlers,
  installWorkflowHandlers,
  TEST_TENANT_ID,
  workflowFixture,
} from '@/test/cloud-handlers'
import { server } from '@/test/msw-server'
import { renderAtRoute } from '@/test/render'

const WORKFLOW_ID = '44444444-4444-4444-4444-444444444444'
const WORKSPACE_SLUG = 'cloud-dev'
const RUNS_URL = `/api/v1/tenants/${TEST_TENANT_ID}/workflows/${WORKFLOW_ID}/runs`
const RUNS_PATH = `/w/${WORKSPACE_SLUG}/workflows/${WORKFLOW_ID}/runs`

/** A run row the history endpoints return; ids stay distinct so a repeat would be visible. */
function runFixture(index: number, overrides: Partial<WorkflowRun> = {}): WorkflowRun {
  return {
    id: `run-${String(index).padStart(3, '0')}`,
    tenantId: TEST_TENANT_ID,
    workflowId: WORKFLOW_ID,
    snapshotId: 'snap-1',
    name: `第 ${index} 次运行`,
    workflowName: '安全审查',
    status: 'succeeded',
    input: {},
    nodeStates: {},
    rounds: [],
    error: '',
    definitionSnapshot: null,
    startedAt: '2026-09-20T02:00:00.000Z',
    finishedAt: '2026-09-20T02:00:10.000Z',
    cancelRequestedAt: null,
    pluginVersion: '1.0.0',
    createdAt: '2026-09-20T02:00:00.000Z',
    updatedAt: '2026-09-20T02:00:10.000Z',
    ...overrides,
  }
}

/**
 * Serves a newest-first history of `total` runs, `limit` rows at a time, using the row id as the
 * cursor exactly like the backend's opaque UUID cursor. `after` is exclusive, so a page boundary
 * that repeated or skipped a row would show up as a duplicate or a missing name on screen.
 */
function installHistory(total: number, onCursor?: (after: string) => void): void {
  const runs = Array.from({ length: total }, (_, index) => runFixture(index + 1))
  server.use(
    http.get(RUNS_URL, ({ request }) => {
      const params = new URL(request.url).searchParams
      const after = params.get('after') ?? ''
      onCursor?.(after)
      const limit = Number(params.get('limit') ?? '50')
      const start = after === '' ? 0 : runs.findIndex((run) => run.id === after) + 1
      const page = runs.slice(start, start + limit)
      const more = start + limit < runs.length
      return HttpResponse.json({ items: page, nextCursor: more ? page[page.length - 1]?.id : '' })
    }),
  )
}

/** Mounts the runs tab behind a real route match, so the workspace slug comes from the URL. */
function renderRunsPage() {
  installCloudSpaceHandlers('admin')
  installWorkflowHandlers([workflowFixture({ id: WORKFLOW_ID })])
  return renderAtRoute(
    '/w/:workspaceSlug/workflows/:workflowId/runs',
    <WorkflowRunsPage slug={TEST_TENANT_ID} />,
    RUNS_PATH,
  )
}

/** The run names currently rendered, in display order. */
function renderedRunNames(): string[] {
  return screen
    .getAllByRole('listitem')
    .map((row) => within(row).getByRole('link').getAttribute('aria-label') ?? '')
}

/**
 * Presses "load older" until the list holds `total` rows.
 *
 * Recursive rather than a loop because each step has to wait for the page to render before the
 * next click: awaiting inside a loop would race the render it just triggered.
 */
async function loadEveryPage(total: number): Promise<void> {
  const loaded = screen.getAllByRole('listitem').length
  if (loaded >= total) return
  fireEvent.click(screen.getByRole('button', { name: '加载更早的运行' }))
  await waitFor(() => expect(screen.getAllByRole('listitem').length).toBeGreaterThan(loaded))
  await loadEveryPage(total)
}

describe('WorkflowRunsPage', () => {
  it('lists the newest run first and links into its detail page', async () => {
    installHistory(3)
    renderRunsPage()

    await screen.findByText('第 1 次运行')
    expect(renderedRunNames()).toEqual([
      '打开运行 第 1 次运行',
      '打开运行 第 2 次运行',
      '打开运行 第 3 次运行',
    ])
    const firstRow = screen.getAllByRole('listitem')[0]
    expect(firstRow === undefined ? null : within(firstRow).getByRole('link')).toHaveAttribute(
      'href',
      `${RUNS_PATH}/run-001`,
    )
  })

  it('says the workflow has no runs instead of showing an empty list', async () => {
    installHistory(0)
    renderRunsPage()

    expect(await screen.findByText('还没有运行记录')).toBeInTheDocument()
    expect(screen.queryByRole('listitem')).not.toBeInTheDocument()
  })

  it('offers a retry when the history cannot be read, rather than calling it empty', async () => {
    server.use(http.get(RUNS_URL, () => HttpResponse.json({ code: 'internal' }, { status: 500 })))
    renderRunsPage()

    expect(await screen.findByText('运行记录读取失败')).toBeInTheDocument()
    expect(screen.queryByText('还没有运行记录')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: '重试' })).toBeInTheDocument()
  })

  it('reports a missing permission as its own state', async () => {
    server.use(http.get(RUNS_URL, () => HttpResponse.json({ code: 'forbidden' }, { status: 403 })))
    renderRunsPage()

    expect(await screen.findByText('没有查看运行记录的权限')).toBeInTheDocument()
    expect(screen.queryByText('运行记录读取失败')).not.toBeInTheDocument()
  })

  it('loads every page past the first hundred runs without repeating or skipping one', async () => {
    // The old surface asked for one page of 100 and stopped; a history longer than that has to
    // stay fully reachable, so this walks the whole thing the way a member would.
    const total = 120
    const cursors: string[] = []
    installHistory(total, (after) => cursors.push(after))
    renderRunsPage()

    await screen.findByText('第 1 次运行')
    expect(screen.getAllByRole('listitem')).toHaveLength(WORKFLOW_RUN_PAGE_SIZE)

    await loadEveryPage(total)

    expect(renderedRunNames()).toHaveLength(total)
    expect(new Set(renderedRunNames()).size).toBe(total)
    expect(renderedRunNames()[0]).toBe('打开运行 第 1 次运行')
    expect(renderedRunNames()[total - 1]).toBe(`打开运行 第 ${total} 次运行`)
    // Each request continued below the row the previous page ended on: the first asks for the head,
    // every later one carries a cursor, and no cursor is repeated.
    expect(cursors[0]).toBe('')
    expect(new Set(cursors.slice(1)).size).toBe(cursors.length - 1)
    expect(await screen.findByText('已经到最早的一次运行。')).toBeInTheDocument()
  })

  it('keeps the rows it already loaded when a later page fails', async () => {
    // The head answers with two rows and a cursor; every later page fails. The loaded history
    // must survive, and the failure has to read as a failed read rather than as the end of it.
    server.use(
      http.get(RUNS_URL, ({ request }) => {
        const after = new URL(request.url).searchParams.get('after') ?? ''
        if (after === '') {
          return HttpResponse.json({
            items: [runFixture(1), runFixture(2)],
            nextCursor: 'run-002',
          })
        }
        return HttpResponse.json({ code: 'internal' }, { status: 500 })
      }),
    )
    renderRunsPage()
    await screen.findByText('第 1 次运行')

    fireEvent.click(screen.getByRole('button', { name: '加载更早的运行' }))

    expect(await screen.findByText('更早的运行读取失败。')).toBeInTheDocument()
    expect(screen.getByText('第 1 次运行')).toBeInTheDocument()
    expect(screen.getByText('第 2 次运行')).toBeInTheDocument()
    // The control that failed is still there, so the retry is the same button.
    expect(screen.getByRole('button', { name: '加载更早的运行' })).toBeInTheDocument()
  })
})
