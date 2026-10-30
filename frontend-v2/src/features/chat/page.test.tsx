import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import Page from './page'
import { replayClock, resetChatStore, useChatStore } from './store'
import { type ChatBackend, chatBackend, FakeEventSource, line } from './test-backend'

const renderPage = () =>
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <TooltipProvider>
        <Page />
      </TooltipProvider>
    </QueryClientProvider>,
  )
const sidebar = () => screen.getByRole('complementary', { name: '聊天记录' })
/** Renders the page and waits until it has loaded the backend's projects. */
const renderReady = async () => {
  renderPage()
  await within(sidebar()).findByRole('region', { name: '项目 ora-space/cloud' })
}

describe('chat page', () => {
  let backend: ChatBackend
  beforeEach(() => {
    replayClock.scale = 0
    resetChatStore()
    backend = chatBackend()
  })
  afterEach(() => {
    replayClock.scale = 1
  })

  it('groups history by project', async () => {
    await renderReady()
    expect(screen.getByRole('heading', { level: 1, name: '对话' })).toBeInTheDocument()
    const cloud = within(sidebar()).getByRole('region', { name: '项目 ora-space/cloud' })
    expect(within(cloud).getByText('梳理 v2 接口缺口')).toBeInTheDocument()
    expect(within(cloud).getByText('对话页样式')).toBeInTheDocument()
  })

  it('opens on a new chat in the first project, as desktop lands', async () => {
    await renderReady()
    expect(screen.getByRole('heading', { level: 2, name: '新对话' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { level: 2, name: '今天想构建什么？' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '选择项目' })).toHaveTextContent('ora-space/cloud')
    // Every conversation gets its own directory: there is no workspace to choose.
    expect(screen.queryByRole('button', { name: '选择工作目录' })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: '选择启动模式' })).toHaveTextContent('云端')
  })

  it('opens a sample from the history and shows its work', async () => {
    renderPage()
    await userEvent.click(within(sidebar()).getByRole('button', { name: '工具调用：读、搜、改、执行' }))
    expect(screen.getByRole('heading', { level: 2, name: '工具调用：读、搜、改、执行' })).toBeInTheDocument()
    expect(screen.getByText('把对话页的标题改成"对话"，然后跑一下检查')).toBeInTheDocument()
    expect(screen.getByText('部分操作失败')).toBeInTheDocument()
    expect(screen.getByText('修改了 1 个文件')).toBeInTheDocument()
    // The summary opens into the individual steps.
    await userEvent.click(screen.getByText('部分操作失败'))
    expect(screen.getByText('运行 npm run e2e')).toBeInTheDocument()
    expect(screen.getByText(/PostgreSQL is not reachable/)).toBeInTheDocument()
  })

  it('asks for permission inline and continues with the answer', async () => {
    renderPage()
    await userEvent.click(within(sidebar()).getByRole('button', { name: '权限确认' }))
    const card = screen.getByRole('group', { name: '权限确认' })
    await userEvent.click(within(card).getByRole('button', { name: '允许一次' }))
    expect(screen.queryByRole('group', { name: '权限确认' })).not.toBeInTheDocument()
    expect(screen.getByText('你选择了：允许一次')).toBeInTheDocument()
    expect(screen.getByText('依赖已经重新安装，共 412 个包。')).toBeInTheDocument()
  })

  it('keeps plugins and files for samples, where the input can be tried without the backend', async () => {
    renderPage()
    await userEvent.click(within(sidebar()).getByRole('button', { name: '工具调用：读、搜、改、执行' }))
    const input = screen.getByRole('textbox', { name: '消息' })
    await userEvent.type(input, '检查一下 @代码')
    await userEvent.click(screen.getByRole('option', { name: '代码审查' }))
    await userEvent.type(input, '{Enter}')
    expect(screen.getByText(/插件：代码审查/)).toBeInTheDocument()
  })

  it('starts a conversation in a project with a text prompt, and shows the Agent\'s reply', async () => {
    await renderReady()
    await userEvent.click(within(sidebar()).getByRole('button', { name: '在 ora-space/desktop 新建对话' }))
    expect(screen.getByRole('button', { name: '选择项目' })).toHaveTextContent('ora-space/desktop')
    // The first version sends text only: no files or plugins.
    expect(screen.queryByRole('button', { name: '添加' })).not.toBeInTheDocument()
    const input = screen.getByRole('textbox', { name: '消息' })
    await userEvent.type(input, '检查一下 @代码{Enter}')
    expect(screen.queryByRole('option', { name: '代码审查' })).not.toBeInTheDocument()
    expect(await screen.findByRole('heading', { level: 2, name: '检查一下 @代码' })).toBeInTheDocument()
    expect(backend.bodies.at(-1)).toEqual({ projectId: 'p-desktop', executorId: 'oc-fixture', prompt: '检查一下 @代码' })
    act(() =>
      FakeEventSource.of('c-new-1').emit({
        kind: 'history_snapshot',
        history: { records: [line.user(1, '检查一下 @代码'), line.say(2, '检查完了，没有问题。'), line.ended(3)], incomplete: false },
      }),
    )
    expect(screen.getByText('检查完了，没有问题。')).toBeInTheDocument()
    expect(within(sidebar()).getByRole('region', { name: '项目 ora-space/desktop' })).toHaveTextContent('检查一下 @代码')
    expect(input).toHaveValue('')
  })

  it('makes a stopped conversation read-only and offers a new chat in its project', async () => {
    await renderReady()
    await userEvent.click(within(sidebar()).getByRole('button', { name: '梳理 v2 接口缺口' }))
    const stream = FakeEventSource.of('c-main')
    act(() => stream.emit({ kind: 'history_snapshot', history: { records: [line.user(1, '写长一点')], incomplete: false } }))
    await userEvent.click(screen.getByRole('button', { name: '停止' }))
    act(() => {
      stream.emit({ kind: 'turn_ended', result: { status: 'cancelled', answer: '', answerComplete: false, stopReason: 'cancelled', failureCode: '' } })
      stream.emit({ kind: 'session_ended' })
    })
    expect(screen.getByText(/已停止 · 这个对话已结束/)).toBeInTheDocument()
    expect(screen.queryByRole('textbox', { name: '消息' })).not.toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: '新对话' }))
    expect(screen.getByRole('heading', { level: 2, name: '今天想构建什么？' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '选择项目' })).toHaveTextContent('ora-space/cloud')
    expect(screen.getByRole('textbox', { name: '消息' })).toBeInTheDocument()
  })

  it('shows where a new chat runs; the cloud is the only mode on the web', async () => {
    await renderReady()
    await userEvent.click(screen.getByRole('button', { name: '选择启动模式' }))
    expect(await screen.findByRole('menuitem', { name: /本地/ })).toHaveAttribute('aria-disabled', 'true')
  })

  it('searches the backend\'s projects, with no way to add one yet', async () => {
    await renderReady()
    await userEvent.click(screen.getByRole('button', { name: '选择项目' }))
    await userEvent.type(await screen.findByPlaceholderText('搜索项目'), 'desk')
    expect(screen.getByRole('option', { name: 'ora-space/desktop' })).toBeInTheDocument()
    expect(screen.queryByRole('option', { name: 'ora-space/cloud' })).not.toBeInTheDocument()
    expect(screen.queryByRole('option', { name: /新建项目/ })).not.toBeInTheDocument()
  })

  it('picks an agent, then one of its models, from the backend\'s executors', async () => {
    await renderReady()
    await userEvent.click(screen.getByRole('button', { name: /^模型：/ }))
    await userEvent.click(await screen.findByRole('menuitem', { name: 'OpenCode' }))
    await userEvent.type(screen.getByRole('textbox', { name: '搜索模型' }), 'sonnet')
    expect(screen.queryByRole('menuitem', { name: 'Fixture' })).not.toBeInTheDocument()
    await userEvent.click(screen.getByRole('menuitem', { name: 'Claude Sonnet 5.5' }))
    expect(screen.getByRole('button', { name: /^模型：/ })).toHaveAccessibleName('模型：OpenCode Claude Sonnet 5.5')
  })

  it('says when the backend cannot run Agents yet', async () => {
    resetChatStore()
    chatBackend({ unavailable: true })
    renderPage()
    expect(await screen.findByText(/后端还不能运行 Agent/)).toHaveRole('status')
  })

  it('labels every chat as working in a worktree of its own, explained on hover', async () => {
    await renderReady()
    const label = screen.getByText('Worktree')
    await userEvent.hover(label)
    expect(await screen.findByText('每个对话都在新建的 worktree 里工作')).toBeInTheDocument()
    await userEvent.unhover(label)
    await userEvent.click(within(sidebar()).getByRole('button', { name: '梳理 v2 接口缺口' }))
    expect(within(screen.getByRole('banner')).getByText('Worktree')).toBeInTheDocument()
  })

  it('renames a conversation by double-clicking it', async () => {
    await renderReady()
    await userEvent.dblClick(within(sidebar()).getByRole('button', { name: '梳理 v2 接口缺口' }))
    const field = screen.getByRole('textbox', { name: '会话名称' })
    await userEvent.clear(field)
    await userEvent.type(field, '接口清单{Enter}')
    expect(within(sidebar()).getByRole('button', { name: '接口清单' })).toBeInTheDocument()
    expect(backend.bodies.at(-1)).toEqual({ title: '接口清单' })
  })

  it('folds the history column away and brings it back', async () => {
    renderPage()
    await userEvent.click(within(sidebar()).getByRole('button', { name: '收起对话列表' }))
    expect(screen.queryByRole('complementary', { name: '聊天记录' })).not.toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: '展开对话列表' }))
    expect(sidebar()).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '展开对话列表' })).not.toBeInTheDocument()
  })

  it('resizes the history column from its edge, folding it past the narrowest width', async () => {
    renderPage()
    const edge = screen.getByRole('separator', { name: '调整对话列表宽度' })
    edge.focus()
    await userEvent.keyboard('{ArrowRight}')
    expect(sidebar()).toHaveStyle({ width: '304px' })
    expect(edge).toHaveAttribute('aria-valuenow', '304')
    act(() => useChatStore.getState().setSidebar({ width: 200 }))
    await userEvent.keyboard('{ArrowLeft}')
    expect(screen.queryByRole('complementary', { name: '聊天记录' })).not.toBeInTheDocument()
    expect(edge).toHaveAttribute('aria-valuenow', '0')
    await userEvent.keyboard('{Enter}')
    expect(sidebar()).toHaveStyle({ width: '200px' })
  })

  it('reorders projects when one is dropped on another', async () => {
    await renderReady()
    const order = () => within(sidebar()).getAllByRole('region').map((r) => r.getAttribute('aria-label'))
    expect(order()).toEqual(['项目 示例', '项目 ora-space/cloud', '项目 ora-space/desktop'])
    // dnd-kit's keyboard sensor needs layout jsdom lacks, so drive the store the drop calls.
    act(() => useChatStore.getState().moveProject('p-desktop', 'samples'))
    expect(order()).toEqual(['项目 ora-space/desktop', '项目 示例', '项目 ora-space/cloud'])
  })
})
