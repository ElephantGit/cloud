import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import type { ThreadEntry } from '@/api/generated.schemas'
import { ThreadMessages } from './thread-messages'

function message(seq: number): ThreadEntry {
  return {
    seq,
    kind: 'update',
    source: 'node',
    turnId: `turn-${seq}`,
    record: {
      update: {
        sessionUpdate: 'agent_message_chunk',
        content: { type: 'text', text: `Reply ${seq}` },
      },
    },
    createdAt: '2026-10-01T00:00:00Z',
  }
}

describe('ThreadMessages reading position', () => {
  it('does not pull readers away from history when a new message arrives', async () => {
    const scroll = vi.spyOn(Element.prototype, 'scrollIntoView')
    const load = vi.fn<() => void>()
    const view = render(
      <ThreadMessages
        entries={[message(1)]}
        hasOlder={false}
        loadingOlder={false}
        onLoadOlder={load}
      />,
    )
    const container = screen.getByRole('list', { name: '会话消息' }).parentElement
    if (!container) throw new Error('Message scroller missing')
    Object.defineProperties(container, {
      scrollHeight: { value: 1000, configurable: true },
      clientHeight: { value: 300, configurable: true },
    })
    fireEvent.scroll(container, { target: { scrollTop: 100 } })
    scroll.mockClear()
    view.rerender(
      <ThreadMessages
        entries={[message(1), message(2)]}
        hasOlder={false}
        loadingOlder={false}
        onLoadOlder={load}
      />,
    )
    expect(scroll).not.toHaveBeenCalled()
    await userEvent.setup().click(screen.getByRole('button', { name: '有新内容 · 回到底部' }))
    expect(scroll).toHaveBeenCalledOnce()
    expect(screen.queryByRole('button', { name: '有新内容 · 回到底部' })).not.toBeInTheDocument()
    scroll.mockRestore()
  })

  it('preserves the viewport anchor when older messages are prepended', async () => {
    const load = vi.fn<() => void>()
    const view = render(
      <ThreadMessages entries={[message(2)]} hasOlder loadingOlder={false} onLoadOlder={load} />,
    )
    const container = screen.getByRole('list', { name: '会话消息' }).parentElement
    if (!container) throw new Error('Message scroller missing')
    Object.defineProperties(container, {
      scrollHeight: { value: 600, configurable: true },
      clientHeight: { value: 200, configurable: true },
    })
    fireEvent.scroll(container, { target: { scrollTop: 40 } })
    await userEvent.setup().click(screen.getByRole('button', { name: '加载更早' }))
    Object.defineProperty(container, 'scrollHeight', { value: 900 })
    view.rerender(
      <ThreadMessages
        entries={[message(1), message(2)]}
        hasOlder={false}
        loadingOlder={false}
        onLoadOlder={load}
      />,
    )
    expect(load).toHaveBeenCalledOnce()
    expect(container.scrollTop).toBe(340)
  })
})
