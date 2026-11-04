import type * as acp from '@agentclientprotocol/sdk'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import { applyEvent } from './model/conversation'
import { type ChatEvent, type Conversation, emptyConversation } from './model/types'
import { Thread } from './thread'
import { applySessionEvent, conversationOf as fromHistory, emptySessionView } from './model/history'
import type { SessionEvent } from './model/wire'

const conversationOf = (events: ChatEvent[]): Conversation => events.reduce((c, e) => applyEvent(c, e, 0), emptyConversation)
const update = (u: acp.SessionUpdate): ChatEvent => ({ type: 'update', update: u })
const say = (text: string) => update({ sessionUpdate: 'agent_message_chunk', content: { type: 'text', text } })
const tool = (toolCallId: string, title: string, kind: acp.ToolKind, rawInput?: unknown) =>
  update({ sessionUpdate: 'tool_call', toolCallId, title, kind, status: 'in_progress', rawInput })
const done = (toolCallId: string, extra: Partial<acp.ToolCallUpdate> = {}) => update({ sessionUpdate: 'tool_call_update', toolCallId, status: 'completed', ...extra })
const fail = (toolCallId: string, text: string) =>
  update({ sessionUpdate: 'tool_call_update', toolCallId, status: 'failed', content: [{ type: 'content', content: { type: 'text', text } }] })

const renderThread = (conversation: Conversation, onAnswer = vi.fn()) =>
  render(
    <TooltipProvider>
      <Thread conversation={conversation} onAnswer={onAnswer} />
    </TooltipProvider>,
  )

describe('Thread preparation', () => {
  const at = (ms: number) => new Date(Date.UTC(2026, 10, 3, 0, 0, 0, ms)).toISOString()
  const prompt: ChatEvent = { type: 'prompt', turnId: 't', text: '你好' }

  it('names the running stage and what already finished', () => {
    const conversation = { ...conversationOf([prompt]), preparation: { stages: [
      { id: 'workspace' as const, startedAt: at(0), endedAt: at(2400) },
      { id: 'agent' as const, startedAt: at(2400) },
    ] } }
    renderThread(conversation)
    expect(screen.getByText('正在启动 Agent…（第 4/5 步）')).toBeTruthy()
    expect(screen.getByText('准备工作目录')).toBeTruthy()
    expect(screen.getByText('2.4 秒')).toBeTruthy()
    expect(screen.queryByText('正在准备…')).toBeNull()
  })

  it('lists every stage up front, with what is left and what was already ready', () => {
    const conversation = { ...conversationOf([prompt]), preparation: { stages: [
      { id: 'runtime' as const, startedAt: at(0), endedAt: at(10) },
      { id: 'workspace' as const, startedAt: at(10) },
    ] } }
    renderThread(conversation)
    expect(screen.getByText('正在准备工作目录…（第 2/5 步）')).toBeTruthy()
    const state = (label: string) => screen.getByText(label).closest('li')?.textContent?.replace(label, '')
    expect(state('启动运行环境')).toBe('已就绪')
    expect(state('准备工作目录')).toBe('进行中')
    expect(state('安装插件')).toBe('等待中')
    expect(state('启动 Agent')).toBe('等待中')
    expect(state('等待首次输出')).toBe('等待中')
  })

  it('does not promise stages a turn that stopped early never reached', async () => {
    const conversation = { ...conversationOf([prompt, { type: 'turnError', message: '失败' }]), preparation: { stages: [
      { id: 'workspace' as const, startedAt: at(0) },
    ] } }
    renderThread(conversation)
    await userEvent.click(screen.getByText('准备'))
    expect(screen.getByText('未完成')).toBeTruthy()
    expect(screen.queryByText('等待中')).toBeNull()
  })

  it('folds into the total and opens into every stage, keeping the ones that were ready', async () => {
    const conversation = { ...conversationOf([prompt, say('好')]), preparation: { stages: [
      { id: 'runtime' as const, startedAt: at(0), endedAt: at(10) },
      { id: 'workspace' as const, startedAt: at(10), endedAt: at(20), reused: true },
      { id: 'agent' as const, startedAt: at(20), endedAt: at(4020) },
      { id: 'response' as const, startedAt: at(4020), endedAt: at(7020) },
    ] } }
    renderThread(conversation)
    await userEvent.click(screen.getByText('首次输出用时 7.0 秒'))
    const state = (label: string) => screen.getByText(label).closest('li')?.textContent?.replace(label, '')
    expect(state('启动运行环境')).toBe('已就绪')
    expect(state('准备工作目录（已预热）')).toBe('已就绪')
    expect(state('启动 Agent')).toBe('4.0 秒')
  })

  it('shows it only on the first turn', () => {
    const conversation = { ...conversationOf([prompt, say('好'), { type: 'turnEnd', stopReason: 'end_turn' }, { type: 'prompt', turnId: 'u', text: '再来' }]), preparation: { stages: [
      { id: 'agent' as const, startedAt: at(0), endedAt: at(1000) },
    ] } }
    renderThread(conversation)
    expect(screen.getAllByText('首次输出用时 1.0 秒')).toHaveLength(1)
    expect(screen.getByText('正在准备…')).toBeTruthy()
  })
})

describe('Thread', () => {
  it('shows an estimated reasoning duration after settling and a cold event replay', () => {
    const record = (sequence: number, sessionUpdate: string, text: string, preview = false) => ({
      sequence, preview, data: { type: 'update', at: '2026-11-03T00:00:10Z', update: { sessionUpdate, content: { type: 'text', text } } },
    })
    const user = record(1, 'user_message_chunk', 'hi')
    const thought = record(2, 'agent_thought_chunk', '先想一想', true)
    const answer = record(3, 'agent_message_chunk', '你好')
    const events: SessionEvent[] = [
      { sequence: 1, occurredAt: '2026-11-03T00:00:02Z', kind: 'history_preview', turnId: 't', history: { records: [user, thought], incomplete: false } },
      { sequence: 2, occurredAt: '2026-11-03T00:00:07Z', kind: 'history_preview', turnId: 't', history: { records: [thought, { ...answer, preview: true }], incomplete: false } },
      { sequence: 3, occurredAt: '2026-11-03T00:00:10Z', kind: 'history_snapshot', turnId: 't', history: {
        records: [user, { ...thought, preview: false }, answer, { sequence: 4, data: { type: 'turnEnded', at: '2026-11-03T00:00:10Z', stop_reason: 'end_turn' } }], incomplete: false,
      } },
    ]
    const folded = fromHistory([events.reduce(applySessionEvent, emptySessionView('s'))])
    renderThread(folded)
    expect(screen.getByText('思考了约 5 秒')).toBeInTheDocument()
    expect(screen.queryByText('思考了 0.0 秒')).not.toBeInTheDocument()
  })

  it('shows reasoning without a fabricated duration when only settled history is available', () => {
    const data = (sessionUpdate: string, text: string) => ({ type: 'update', at: '2026-11-03T00:00:10Z', update: { sessionUpdate, content: { type: 'text', text } } })
    renderThread(fromHistory([{ ...emptySessionView('s'), records: [
      { sequence: 1, data: data('user_message_chunk', 'hi') },
      { sequence: 2, data: data('agent_thought_chunk', '先想一想') },
      { sequence: 3, data: data('agent_message_chunk', '你好') },
      { sequence: 4, data: { type: 'turnEnded', at: '2026-11-03T00:00:10Z', stop_reason: 'end_turn' } },
    ] }]))
    expect(screen.getByText('思考')).toBeInTheDocument()
    expect(screen.queryByText(/思考了/)).not.toBeInTheDocument()
  })

  it('folds a turn\'s tool calls into a summary that opens into the steps', async () => {
    renderThread(
      conversationOf([
        { type: 'prompt', turnId: 't1', text: '把对话页的标题改成"对话"，然后跑一下检查' },
        say('先找到标题在哪里定义。'),
        tool('s1', '搜索 "对话"', 'search', { pattern: '对话', path: 'frontend-v2/src' }),
        done('s1'),
        tool('e1', '编辑 src/features/chat/index.ts', 'edit'),
        done('e1', {
          content: [{ type: 'diff', path: 'src/features/chat/index.ts', oldText: "title: '聊天',\n", newText: "title: '对话',\n" }],
        }),
        tool('x2', '运行 npm run e2e', 'execute', { command: 'npm run e2e' }),
        fail('x2', 'Error: PostgreSQL is not reachable at 127.0.0.1:55432'),
        say('标题已经改好，端到端测试没跑成。'),
        { type: 'turnEnd', stopReason: 'end_turn' },
      ]),
    )
    expect(screen.getByText('把对话页的标题改成"对话"，然后跑一下检查')).toBeInTheDocument()
    expect(screen.getByText('部分操作失败')).toBeInTheDocument()
    expect(screen.getByText('修改了 1 个文件')).toBeInTheDocument()
    await userEvent.click(screen.getByText('部分操作失败'))
    expect(screen.getByText('运行 npm run e2e')).toBeInTheDocument()
    expect(screen.getByText(/PostgreSQL is not reachable/)).toBeInTheDocument()
  })

  it('asks for permission inline at the tool call and reports the answer', async () => {
    const onAnswer = vi.fn()
    const toolCall = { toolCallId: 'rm', title: '运行 rm -rf node_modules && npm ci', kind: 'execute' as const }
    renderThread(
      conversationOf([
        { type: 'prompt', turnId: 't1', text: '把 node_modules 删掉重新装一遍依赖' },
        tool('rm', toolCall.title, 'execute', { command: 'rm -rf node_modules && npm ci' }),
        {
          type: 'permission',
          request: {
            id: 'perm-1',
            toolCall,
            options: [
              { optionId: 'allow', name: '允许一次', kind: 'allow_once' },
              { optionId: 'reject', name: '拒绝', kind: 'reject_once' },
            ],
          },
        },
      ]),
      onAnswer,
    )
    const card = screen.getByRole('group', { name: '权限确认' })
    await userEvent.click(within(card).getByRole('button', { name: '允许一次' }))
    expect(onAnswer).toHaveBeenCalledWith('perm-1', 'allow')
  })

  it('says which option was chosen once a permission is resolved', () => {
    renderThread(
      conversationOf([
        { type: 'prompt', turnId: 't1', text: '删掉重装' },
        tool('rm', '运行 rm -rf node_modules', 'execute'),
        { type: 'permission', request: { id: 'perm-1', toolCall: { toolCallId: 'rm' }, options: [{ optionId: 'allow', name: '允许一次', kind: 'allow_once' }] } },
        { type: 'permissionResolved', id: 'perm-1', optionId: 'allow' },
        done('rm'),
        say('依赖已经重新安装。'),
        { type: 'turnEnd', stopReason: 'end_turn' },
      ]),
    )
    expect(screen.queryByRole('group', { name: '权限确认' })).not.toBeInTheDocument()
    expect(screen.getByText('你选择了：允许一次')).toBeInTheDocument()
    expect(screen.getByText('依赖已经重新安装。')).toBeInTheDocument()
  })
})
