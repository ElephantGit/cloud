import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it, vi } from 'vitest'
import { CreateLinkDialog } from '@/features/members/create-link-dialog'
import { copyJoinLink } from '@/features/members/browser'
import { renderWithProviders } from '@/test/render'
import { server } from '@/test/msw-server'

vi.mock('@/features/members/browser', async (importOriginal) => {
  const original = await importOriginal<Record<string, unknown>>()
  return { ...original, copyJoinLink: vi.fn<typeof copyJoinLink>() }
})

describe('Private link generation dialog', () => {
  it('sends a note, reports copy errors, and confirms before discarding plaintext', async () => {
    const close = vi.fn<() => void>()
    const copy = vi
      .mocked(copyJoinLink)
      .mockRejectedValueOnce(new Error('clipboard unavailable'))
      .mockResolvedValue(undefined)
    let body: unknown
    server.use(
      http.post('/api/v1/tenants/tenant/invitations', async ({ request }) => {
        body = await request.json()
        return HttpResponse.json(
          { id: 'invite', expiresAt: '2099-11-03T00:00:00Z' },
          { status: 201 },
        )
      }),
    )
    renderWithProviders(<CreateLinkDialog tenantId="tenant" kind="invite" onClose={close} />)
    const user = userEvent.setup()
    await user.type(screen.getByLabelText('备注（可选）'), 'Designer onboarding')
    await user.click(screen.getByRole('button', { name: '生成链接' }))
    const input = await screen.findByLabelText<HTMLInputElement>('新生成的链接')
    expect(input.value).toContain('/join/invite/')
    expect(body).toEqual({
      token: expect.stringMatching(/^[A-Za-z0-9_-]{43}$/),
      note: 'Designer onboarding',
    })
    await user.click(screen.getByRole('button', { name: '复制链接' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('复制失败')
    await user.click(screen.getByRole('button', { name: '复制链接' }))
    expect(await screen.findByRole('button', { name: '已复制' })).toBeInTheDocument()
    expect(copy).toHaveBeenCalledTimes(2)
    expect(copy.mock.calls[0]?.[0]).toEqual(copy.mock.calls[1]?.[0])
    await user.click(screen.getByRole('button', { name: '关闭' }))
    expect(close).not.toHaveBeenCalled()
    await user.click(screen.getByRole('button', { name: '返回复制' }))
    expect(screen.getByLabelText('新生成的链接')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '关闭' }))
    await user.click(screen.getByRole('button', { name: '已保存，关闭' }))
    expect(close).toHaveBeenCalledOnce()
    copy.mockRestore()
  })
})
