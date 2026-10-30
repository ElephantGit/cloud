import { beforeEach, expect, it } from 'vitest'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderSettingsPage } from '../test-utils'
import { DemoRepository } from '../data/repository'
import { demoUser } from '@/test/backend'
import MembersPage from './page'

beforeEach(() => localStorage.clear())
it('approves an application and displays its provenance in the roster', async () => {
  const user = userEvent.setup()
  renderSettingsPage(<MembersPage />)
  await user.click(await screen.findByRole('button', { name: /加入申请/ }))
  await user.click(screen.getByRole('button', { name: '批准' }))
  await waitFor(() =>
    expect(
      screen.queryByRole('button', { name: '批准' }),
    ).not.toBeInTheDocument(),
  )
  await user.click(screen.getByRole('button', { name: '成员列表' }))
  const row = await screen.findByRole('row', { name: /许安/ })
  await user.click(within(row).getByRole('button', { name: '加入历史' }))
  expect(await screen.findByRole('dialog')).toHaveTextContent('申请批准')
})
it('creates a local preview link and revokes it without a real invitation request', async () => {
  const user = userEvent.setup()
  renderSettingsPage(<MembersPage />, '/settings/members')
  await user.click(await screen.findByRole('button', { name: '邀请与链接' }))
  await user.click(screen.getByRole('button', { name: '生成演示链接' }))
  await user.type(screen.getByLabelText('备注'), 'QA 邀请')
  await user.click(
    within(screen.getByRole('dialog')).getByRole('button', {
      name: '生成演示链接',
    }),
  )
  expect(
    await screen.findByRole('dialog', { name: '本机演示链接预览' }),
  ).toHaveTextContent('不会让任何人加入真实工作空间')
  const input = screen.getByRole('textbox', { name: '仅本机可用的预览链接' })
  expect(input.getAttribute('value')).toContain('/settings/members?preview=')
  await user.click(screen.getByRole('button', { name: '关闭详情' }))
  await user.click(screen.getByRole('button', { name: '邀请与链接' }))
  const row = screen.getByText('QA 邀请').closest('article')!
  await user.click(within(row).getByRole('button', { name: '撤销' }))
  await user.click(
    within(screen.getByRole('alertdialog')).getByRole('button', {
      name: '撤销链接',
    }),
  )
  await waitFor(() =>
    expect(within(row).getByText('已撤销')).toBeInTheDocument(),
  )
})
it('does not expose administration or history to a readonly demo member', async () => {
  const repo = new DemoRepository(demoUser)
  const data = await repo.read()
  await repo.update(data.revision, (next) => {
    next.members.find((m) => m.userId === demoUser.id)!.role = 'member'
  })
  renderSettingsPage(<MembersPage />)
  await screen.findByText('当前演示身份为只读成员，管理操作仅向管理员开放。')
  expect(
    screen.queryByRole('button', { name: /加入申请|停用|加入历史/ }),
  ).not.toBeInTheDocument()
})
