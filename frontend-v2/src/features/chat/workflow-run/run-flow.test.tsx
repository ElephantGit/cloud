import '@/lib/workflows/test/dom'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import Page from '../page'
import { resetChatStore } from '../store'
import { demoRunClock, resetDemoRuns } from './demo-runs'

const renderPage = () =>
  render(
    <TooltipProvider>
      <Page />
    </TooltipProvider>,
  )
const sidebar = () => screen.getByRole('complementary', { name: '聊天记录' })
const project = () => within(sidebar()).getByRole('region', { name: '项目 ora-space/cloud' })

/** Creates a run of the review sample from the project's + menu, as a person would. */
async function createRun(user: ReturnType<typeof userEvent.setup>, name: string) {
  await user.click(within(project()).getByRole('button', { name: '在 ora-space/cloud 新建对话' }))
  await user.click(await screen.findByRole('menuitem', { name: '运行工作流' }))
  // A pointer leaving the submenu's trigger closes the submenu in jsdom, so the item is clicked in place.
  fireEvent.click(await screen.findByRole('menuitem', { name: /示例：代码审查/ }))
  const dialog = await screen.findByRole('dialog', { name: '运行工作流' })
  const field = within(dialog).getByLabelText('运行名称')
  await user.clear(field)
  await user.type(field, name)
  await user.click(within(dialog).getByRole('button', { name: '创建运行' }))
}

describe('workflow runs in chat', () => {
  beforeEach(() => {
    window.localStorage.clear()
    resetChatStore()
    resetDemoRuns()
    demoRunClock.nodeStepMs = 5
  })
  afterEach(() => {
    resetDemoRuns()
    demoRunClock.nodeStepMs = 1500
  })

  it('runs a published workflow from a project and shows each node’s work', async () => {
    const user = userEvent.setup()
    renderPage()
    await createRun(user, '审查 cloud#210')

    // The run opens pending in the main area and is listed with the project's sessions.
    const pane = await screen.findByRole('region', { name: '工作流运行' })
    // The run view is loaded on first use.
    expect(await within(pane).findByRole('heading', { level: 2, name: '审查 cloud#210' })).toBeInTheDocument()
    expect(within(pane).getByText('演示运行')).toBeInTheDocument()
    expect(within(project()).getByRole('button', { name: '工作流运行 审查 cloud#210（未运行）' })).toHaveAttribute('aria-current', 'page')

    await user.click(within(pane).getByRole('button', { name: '启动' }))
    const start = await screen.findByRole('dialog')
    await user.click(within(start).getByRole('button', { name: '启动' }))

    await waitFor(() => expect(within(project()).getByRole('button', { name: '工作流运行 审查 cloud#210（成功）' })).toBeInTheDocument(), { timeout: 25_000 })
    const result = within(pane).getByRole('region', { name: '运行结果' })
    expect(result).toHaveTextContent('运行结果 · 成功')
    expect(within(pane).getByRole('button', { name: '再跑一次' })).toBeInTheDocument()
    // jsdom re-renders the canvas on every node step, so the whole run takes seconds here.
  }, 30_000)

  it('goes back to the chat when a session is picked', async () => {
    const user = userEvent.setup()
    renderPage()
    await createRun(user, '临时运行')
    await within(await screen.findByRole('region', { name: '工作流运行' })).findByRole('heading', { level: 2, name: '临时运行' })

    await user.click(within(project()).getByRole('button', { name: '梳理 v2 接口缺口' }))
    expect(screen.queryByRole('region', { name: '工作流运行' })).not.toBeInTheDocument()
    expect(screen.getByRole('heading', { level: 2, name: '梳理 v2 接口缺口' })).toBeInTheDocument()
    expect(within(project()).getByRole('button', { name: '工作流运行 临时运行（未运行）' })).not.toHaveAttribute('aria-current')
  })
})
