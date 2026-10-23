import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { expect, it } from 'vitest'
import { ConversationViewport } from './conversation-viewport'

it('follows output only at the bottom and preserves an anchored message when history is prepended', async () => {
  const { rerender } = render(
    <ConversationViewport>
      <p key="current" data-message-anchor>
        原来的消息
      </p>
    </ConversationViewport>,
  )
  const viewport = screen.getByLabelText('对话滚动区域')
  let height = 1000
  let anchorTop = 20
  Object.defineProperties(viewport, {
    clientHeight: { configurable: true, get: () => 400 },
    scrollHeight: { configurable: true, get: () => height },
  })
  const anchor = screen.getByText('原来的消息')
  anchor.getBoundingClientRect = () => new DOMRect(0, anchorTop, 200, 80)
  viewport.scrollTop = 300
  fireEvent.scroll(viewport)
  height = 1100
  anchorTop = 120
  rerender(
    <ConversationViewport>
      <p key="older" data-message-anchor>
        更早的消息
      </p>
      <p key="current" data-message-anchor>
        原来的消息
      </p>
    </ConversationViewport>,
  )
  await waitFor(() =>
    expect(screen.getByRole('button', { name: '有新内容 · 回到底部' })).toBeInTheDocument(),
  )
  // Prepending 100px keeps the original message at the same viewport position.
  expect(viewport.scrollTop).toBe(400)
  fireEvent.click(screen.getByRole('button', { name: '有新内容 · 回到底部' }))
  expect(viewport.scrollTop).toBe(1100)
  expect(screen.queryByRole('button')).not.toBeInTheDocument()
})

it('does not auto-follow an execution list when its content changes', async () => {
  const { rerender } = render(
    <ConversationViewport followOutput={false}>
      <p>执行记录</p>
    </ConversationViewport>,
  )
  const viewport = screen.getByLabelText('对话滚动区域')
  Object.defineProperties(viewport, {
    clientHeight: { get: () => 400 },
    scrollHeight: { get: () => 1000 },
  })
  viewport.scrollTop = 80
  await act(async () => {
    rerender(
      <ConversationViewport followOutput={false}>
        <p>执行记录</p>
        <p>新增运行</p>
      </ConversationViewport>,
    )
  })
  expect(viewport.scrollTop).toBe(80)
  expect(screen.queryByRole('button', { name: '有新内容 · 回到底部' })).not.toBeInTheDocument()
})
