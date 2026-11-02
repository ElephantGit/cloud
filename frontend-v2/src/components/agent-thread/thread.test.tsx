import type * as acp from '@agentclientprotocol/sdk'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import { applyEvent } from './model/conversation'
import { type ChatEvent, type Conversation, emptyConversation } from './model/types'
import { Thread } from './thread'

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

describe('Thread', () => {
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
