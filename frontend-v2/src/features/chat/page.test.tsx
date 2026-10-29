import { act, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import Page from './page'
import { replayClock, resetChatStore, useChatStore } from './store'

const renderPage = () =>
  render(
    <TooltipProvider>
      <Page />
    </TooltipProvider>,
  )
const sidebar = () => screen.getByRole('complementary', { name: '聊天记录' })

describe('chat page', () => {
  beforeEach(() => {
    replayClock.scale = 0
    resetChatStore()
  })
  afterEach(() => {
    replayClock.scale = 1
  })

  it('groups history by project, with worktree sessions under their branch', () => {
    renderPage()
    expect(screen.getByRole('heading', { level: 1, name: '对话' })).toBeInTheDocument()
    const cloud = within(sidebar()).getByRole('region', { name: '项目 ora-space/cloud' })
    expect(within(cloud).getByText('梳理 v2 接口缺口')).toBeInTheDocument()
    const worktree = within(cloud).getByText('feat/766-v2-chat').closest('[aria-label="worktree feat/766-v2-chat"]') as HTMLElement
    expect(within(worktree).getByText('对话页样式')).toBeInTheDocument()
    expect(within(worktree).getByText('左栏拖动排序')).toBeInTheDocument()
  })

  it('opens on a new chat in the first project, as desktop lands', () => {
    renderPage()
    expect(screen.getByRole('heading', { level: 2, name: '新对话' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { level: 2, name: '今天想构建什么？' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '选择项目' })).toHaveTextContent('ora-space/cloud')
    expect(screen.getByRole('button', { name: '选择工作目录' })).toHaveTextContent('主目录')
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

  it('sends a new chat with a plugin picked by typing @, creating its session', async () => {
    renderPage()
    await userEvent.click(within(sidebar()).getByRole('button', { name: '在 ora-space/desktop 新建对话' }))
    await userEvent.click(await screen.findByRole('menuitem', { name: '在主目录新建对话' }))
    expect(screen.getByRole('button', { name: '选择项目' })).toHaveTextContent('ora-space/desktop')
    const input = screen.getByRole('textbox', { name: '消息' })
    await userEvent.type(input, '检查一下 @代码')
    await userEvent.click(screen.getByRole('option', { name: '代码审查' }))
    expect(within(screen.getByLabelText('已选插件')).getByText('代码审查')).toBeInTheDocument()
    await userEvent.type(input, '{Enter}')
    // The first prompt creates the session and names it; the reply echoes the prompt and its plugin.
    expect(screen.getByRole('heading', { level: 2, name: '检查一下' })).toBeInTheDocument()
    expect(within(sidebar()).getByRole('region', { name: '项目 ora-space/desktop' })).toHaveTextContent('检查一下')
    expect(screen.getByText(/这是演示回复/)).toBeInTheDocument()
    expect(screen.getByText(/插件：代码审查/)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '选择项目' })).not.toBeInTheDocument()
    expect(input).toHaveValue('')
  })

  it('starts a new chat where the selected session works, and moves it with the bar above the input', async () => {
    renderPage()
    await userEvent.click(within(sidebar()).getByRole('button', { name: '对话页样式' }))
    await userEvent.click(within(sidebar()).getByRole('button', { name: '新建对话' }))
    const where = () => screen.getByRole('button', { name: '选择工作目录' })
    expect(screen.getByRole('button', { name: '选择项目' })).toHaveTextContent('ora-space/cloud')
    expect(where()).toHaveTextContent('feat/766-v2-chat')
    await userEvent.click(where())
    await userEvent.click(await screen.findByRole('option', { name: '主目录' }))
    expect(where()).toHaveTextContent('主目录')
    await userEvent.click(screen.getByRole('button', { name: '选择启动模式' }))
    expect(await screen.findByRole('menuitem', { name: /本地/ })).toHaveAttribute('aria-disabled', 'true')
  })

  it('works in a new worktree named after the first prompt', async () => {
    renderPage()
    const where = () => screen.getByRole('button', { name: '选择工作目录' })
    await userEvent.click(where())
    await userEvent.click(await screen.findByRole('option', { name: '在 worktree 中工作' }))
    // No name until the chat starts.
    expect(where()).toHaveTextContent('新 worktree')
    await userEvent.type(screen.getByRole('textbox', { name: '消息' }), 'add retry to the SSE stream{Enter}')
    expect(screen.getByTitle('Agent 在独立的 worktree 里工作')).toHaveTextContent('ora/add-retry-sse-stream')
    const cloud = within(sidebar()).getByRole('region', { name: '项目 ora-space/cloud' })
    expect(within(cloud).getByLabelText('worktree ora/add-retry-sse-stream')).toHaveTextContent('add retry to the SSE')
  })

  it('searches projects and adds one from its folder', async () => {
    renderPage()
    await userEvent.click(screen.getByRole('button', { name: '选择项目' }))
    await userEvent.type(await screen.findByPlaceholderText('搜索项目'), 'desk')
    expect(screen.getByRole('option', { name: 'ora-space/desktop' })).toBeInTheDocument()
    expect(screen.queryByRole('option', { name: 'ora-space/cloud' })).not.toBeInTheDocument()
    await userEvent.clear(screen.getByPlaceholderText('搜索项目'))
    await userEvent.click(screen.getByRole('option', { name: /新建项目/ }))
    await userEvent.type(await screen.findByRole('textbox', { name: '项目文件夹' }), '/workspace/my-app')
    expect(screen.getByText('项目名：my-app')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: '添加项目' }))
    expect(screen.getByRole('button', { name: '选择项目' })).toHaveTextContent('my-app')
    expect(within(sidebar()).getByRole('region', { name: '项目 my-app' })).toHaveTextContent('还没有会话')
  })

  it('picks an agent, then one of its models found by search', async () => {
    renderPage()
    await userEvent.click(screen.getByRole('button', { name: /^模型：/ }))
    await userEvent.click(await screen.findByRole('menuitem', { name: 'Codex' }))
    // The menu stays open on the new agent's models.
    expect(screen.getByRole('menuitem', { name: 'GPT-5.5' })).toBeInTheDocument()
    await userEvent.type(screen.getByRole('textbox', { name: '搜索模型' }), 'mini')
    expect(screen.queryByRole('menuitem', { name: 'GPT-5.5' })).not.toBeInTheDocument()
    await userEvent.click(screen.getByRole('menuitem', { name: 'GPT-5.5 Mini' }))
    expect(screen.getByRole('button', { name: /^模型：/ })).toHaveAccessibleName('模型：Codex GPT-5.5 Mini')
  })

  it('shows where the session works', async () => {
    renderPage()
    await userEvent.click(within(sidebar()).getByRole('button', { name: '对话页样式' }))
    expect(screen.getByTitle('Agent 在独立的 worktree 里工作')).toHaveTextContent('feat/766-v2-chat')
    await userEvent.click(within(sidebar()).getByRole('button', { name: '梳理 v2 接口缺口' }))
    expect(screen.getByTitle('Agent 直接在项目主目录里工作')).toHaveTextContent('主目录')
  })

  it('renames a session by double-clicking it', async () => {
    renderPage()
    await userEvent.dblClick(within(sidebar()).getByRole('button', { name: '梳理 v2 接口缺口' }))
    const field = screen.getByRole('textbox', { name: '会话名称' })
    await userEvent.clear(field)
    await userEvent.type(field, '接口清单{Enter}')
    expect(within(sidebar()).getByRole('button', { name: '接口清单' })).toBeInTheDocument()
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

  it('reorders projects when one is dropped on another', () => {
    renderPage()
    const order = () => within(sidebar()).getAllByRole('region').map((r) => r.getAttribute('aria-label'))
    expect(order()).toEqual(['项目 示例', '项目 ora-space/cloud', '项目 ora-space/desktop'])
    // dnd-kit's keyboard sensor needs layout jsdom lacks, so drive the store the drop calls.
    act(() => useChatStore.getState().moveProject('desktop', 'samples'))
    expect(order()).toEqual([
      '项目 ora-space/desktop',
      '项目 示例',
      '项目 ora-space/cloud',
    ])
  })
})
