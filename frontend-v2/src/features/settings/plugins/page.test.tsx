import { beforeEach, expect, it } from 'vitest'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderSettingsPage } from '../test-utils'
import PluginsPage from './page'

beforeEach(() => localStorage.clear())
it('searches and filters plugins, opens real details, and confirms demo removal', async () => {
  const user = userEvent.setup()
  renderSettingsPage(<PluginsPage />, '/settings/plugins')
  const row = await screen.findByRole('article', { name: '写作工具箱' })
  await user.type(screen.getByRole('textbox', { name: '搜索插件' }), '写作')
  expect(
    screen.queryByRole('article', { name: 'GitHub 工具' }),
  ).not.toBeInTheDocument()
  await user.click(
    within(row).getByRole('button', { name: '查看插件 写作工具箱' }),
  )
  expect(await screen.findByRole('dialog')).toHaveTextContent(
    'demo/writing-kit',
  )
  expect(screen.queryByRole('button', { name: '启用' })).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: '关闭详情' }))
  await user.click(within(row).getByRole('button', { name: '模拟移除' }))
  await user.click(
    within(await screen.findByRole('alertdialog')).getByRole('button', {
      name: '取消',
    }),
  )
  expect(within(row).getByText('已安装')).toBeInTheDocument()
  await user.click(within(row).getByRole('button', { name: '模拟移除' }))
  await user.click(
    within(await screen.findByRole('alertdialog')).getByRole('button', {
      name: '模拟移除',
    }),
  )
  await waitFor(() =>
    expect(within(row).getByText('未安装')).toBeInTheDocument(),
  )
  await user.click(within(row).getByRole('button', { name: '模拟安装' }))
  await waitFor(() =>
    expect(within(row).getByText('已安装')).toBeInTheDocument(),
  )
})
it('clears no-result filters and retries a failed demo installation', async () => {
  const user = userEvent.setup()
  renderSettingsPage(<PluginsPage />)
  const row = await screen.findByRole('article', { name: '浏览器工具' })
  await user.click(within(row).getByRole('button', { name: '重试演示安装' }))
  await waitFor(() =>
    expect(within(row).getByText('已安装')).toBeInTheDocument(),
  )
  await user.type(
    screen.getByRole('textbox', { name: '搜索插件' }),
    'nothing-matches',
  )
  await user.click(screen.getByRole('button', { name: '清除筛选' }))
  expect(screen.getAllByRole('article')).toHaveLength(6)
})
