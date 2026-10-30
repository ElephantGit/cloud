import { beforeEach, expect, it } from 'vitest'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderSettingsPage } from '../test-utils'
import ModelConnectionsPage from './page'

beforeEach(() => localStorage.clear())
it('creates model metadata, simulates credential readiness and saves a default without a key field', async () => {
  const user = userEvent.setup()
  renderSettingsPage(<ModelConnectionsPage />)
  await screen.findByRole('article', { name: '团队日常模型 · 示例' })
  await user.click(screen.getByRole('button', { name: '新建连接' }))
  await user.type(screen.getByLabelText('连接名称'), '新连接')
  await user.type(
    screen.getByLabelText('服务地址'),
    'https://model.example.com/v1',
  )
  await user.type(screen.getByLabelText('模型标识'), 'org/model')
  await user.type(screen.getByLabelText('显示名称'), '审查模型')
  await user.click(screen.getByRole('button', { name: '保存连接' }))
  const row = await screen.findByRole('article', { name: '新连接' })
  expect(screen.queryByLabelText('API Key')).not.toBeInTheDocument()
  await user.click(within(row).getByRole('button', { name: '模拟配置凭据' }))
  await waitFor(() =>
    expect(within(row).getByText('演示凭据已配置')).toBeInTheDocument(),
  )
  await user.selectOptions(
    screen.getByLabelText('默认模型'),
    screen.getByRole('option', { name: '新连接 / 审查模型' }),
  )
  await user.click(screen.getByRole('button', { name: '保存默认模型' }))
  await waitFor(() =>
    expect(screen.getByRole('button', { name: '保存默认模型' })).toBeDisabled(),
  )
  await user.click(within(row).getByRole('switch'))
  await screen.findByText(
    '当前默认选择不可用。请配置演示凭据、启用连接后重新选择。',
  )
})
it('keeps the connection when deletion is cancelled', async () => {
  const user = userEvent.setup()
  renderSettingsPage(<ModelConnectionsPage />)
  const row = await screen.findByRole('article', {
    name: '团队日常模型 · 示例',
  })
  await user.click(within(row).getByRole('button', { name: '删除' }))
  await user.click(
    within(screen.getByRole('alertdialog')).getByRole('button', {
      name: '取消',
    }),
  )
  expect(row).toBeInTheDocument()
})
