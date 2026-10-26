import { act, fireEvent, screen, waitFor, within } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { beforeEach, describe, expect, it } from 'vitest'
import { CurrentSpaceProvider } from '@/features/spaces/current-space'
import { WorkflowEditorPage } from '@/features/workflows/workflow-editor-page'
import { WorkflowRunsPage } from '@/features/workflows/workflow-runs-page'
import { WorkflowVersionsPage } from '@/features/workflows/workflow-versions-page'
import { isJsonRecord } from '@/features/workflows/runtime/json-record'
import {
  installCloudSpaceHandlers,
  installSpaceProjectHandlers,
  TEST_TENANT_ID,
  workflowFixture,
} from '@/test/cloud-handlers'
import { server } from '@/test/msw-server'
import { renderRoutes } from '@/test/render'
import { stubViewportGeometry } from '@/test/viewport'

const WORKFLOW_ID = '44444444-4444-4444-4444-444444444444'
const BASE = `/api/v1/tenants/${TEST_TENANT_ID}/workflows/${WORKFLOW_ID}`
const PATH = `/w/cloud-dev/workflows/${WORKFLOW_ID}`

beforeEach(() => stubViewportGeometry())

describe('workflow draft navigation', () => {
  it.each([
    { entry: '运行记录标签', label: '运行记录', destination: 'runs' },
    { entry: '版本标签', label: '版本', destination: 'versions' },
    { entry: '运行记录工具栏', label: '查看这个工作流的历次运行记录', destination: 'runs' },
    { entry: '版本工具栏', label: '查看已发布的版本并回滚', destination: 'versions' },
  ])(
    'keeps the edited canvas through $entry until its save completes',
    async ({ entry, label, destination }) => {
      installCloudSpaceHandlers('admin')
      installSpaceProjectHandlers()
      let stored = workflowFixture({ id: WORKFLOW_ID })
      let releaseSave: (() => void) | undefined
      const saveGate = new Promise<void>((resolve) => {
        releaseSave = () => resolve()
      })
      let writes = 0
      server.use(
        http.get(BASE, () => HttpResponse.json(stored)),
        http.get(`${BASE}/snapshots`, () => HttpResponse.json({ items: [], nextCursor: '' })),
        http.get(`${BASE}/runs`, () => HttpResponse.json({ items: [], nextCursor: '' })),
        http.put(BASE, async ({ request }) => {
          const body: unknown = await request.json()
          if (!isJsonRecord(body) || !isJsonRecord(body['graph'])) {
            return HttpResponse.json({ code: 'invalid_request' }, { status: 400 })
          }
          writes += 1
          await saveGate
          stored = workflowFixture({ id: WORKFLOW_ID, graph: body['graph'], version: 2 })
          return HttpResponse.json(stored)
        }),
      )
      const { router } = renderRoutes(
        [
          {
            path: '/w/:workspaceSlug/workflows/:workflowId',
            element: (
              <CurrentSpaceProvider slug="cloud-dev">
                <WorkflowEditorPage slug={TEST_TENANT_ID} />
              </CurrentSpaceProvider>
            ),
          },
          {
            path: '/w/:workspaceSlug/workflows/:workflowId/runs',
            element: (
              <CurrentSpaceProvider slug="cloud-dev">
                <WorkflowRunsPage slug={TEST_TENANT_ID} />
              </CurrentSpaceProvider>
            ),
          },
          {
            path: '/w/:workspaceSlug/workflows/:workflowId/versions',
            element: (
              <CurrentSpaceProvider slug="cloud-dev">
                <WorkflowVersionsPage slug={TEST_TENANT_ID} />
              </CurrentSpaceProvider>
            ),
          },
        ],
        PATH,
      )

      const palette = await screen.findByRole('toolbar', { name: '添加工作流节点' })
      fireEvent.click(within(palette).getByRole('button', { name: '输出' }))
      expect(screen.getByText(/1 个节点/)).toBeInTheDocument()

      const link = entry.endsWith('标签')
        ? within(screen.getByRole('navigation', { name: '工作流的分区' })).getByRole('link', {
            name: label,
          })
        : screen.getByTitle(label)
      fireEvent.click(link)
      await waitFor(() => expect(writes).toBe(1))
      expect(router.state.location.pathname).toBe(PATH)
      expect(screen.getByText(/1 个节点/)).toBeInTheDocument()

      await act(async () => releaseSave?.())
      await waitFor(() => expect(router.state.location.pathname).toBe(`${PATH}/${destination}`))
      fireEvent.click(
        within(screen.getByRole('navigation', { name: '工作流的分区' })).getByRole('link', {
          name: '编辑',
        }),
      )
      await screen.findByRole('toolbar', { name: '添加工作流节点' })
      expect(stored.graph).toMatchObject({
        nodes: [expect.objectContaining({ id: 'output-1' })],
      })
      expect(screen.getByText(/1 个节点/)).toBeInTheDocument()
    },
  )

  it('keeps the editor open when the draft save fails', async () => {
    installCloudSpaceHandlers('admin')
    installSpaceProjectHandlers()
    server.use(
      http.get(BASE, () => HttpResponse.json(workflowFixture({ id: WORKFLOW_ID }))),
      http.get(`${BASE}/snapshots`, () => HttpResponse.json({ items: [], nextCursor: '' })),
      http.put(BASE, () => HttpResponse.json({ code: 'workflow_conflict' }, { status: 409 })),
    )
    const { router, unmount } = renderRoutes(
      [
        {
          path: '/w/:workspaceSlug/workflows/:workflowId',
          element: (
            <CurrentSpaceProvider slug="cloud-dev">
              <WorkflowEditorPage slug={TEST_TENANT_ID} />
            </CurrentSpaceProvider>
          ),
        },
        { path: '/w/:workspaceSlug/workflows/:workflowId/runs', element: <p>运行记录已打开</p> },
      ],
      PATH,
    )
    const palette = await screen.findByRole('toolbar', { name: '添加工作流节点' })
    fireEvent.click(within(palette).getByRole('button', { name: '输出' }))
    fireEvent.click(
      within(screen.getByRole('navigation', { name: '工作流的分区' })).getByRole('link', {
        name: '运行记录',
      }),
    )

    await screen.findByText('保存失败')
    expect(router.state.location.pathname).toBe(PATH)
    expect(screen.getByText(/1 个节点/)).toBeInTheDocument()
    expect(screen.queryByText('运行记录已打开')).not.toBeInTheDocument()
    await act(async () => unmount())
  })
})
