import { beforeEach, expect, it } from 'vitest'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderSettingsPage } from '../test-utils'
import RolesPage from './page'

beforeEach(() => localStorage.clear())

it('creates, renders, edits and deletes a local role', async () => {
  const user = userEvent.setup()
  renderSettingsPage(<RolesPage />)
  await screen.findByRole('article', { name: '代码审查' })
  await user.click(screen.getByRole('button', { name: '新建角色' }))
  await user.type(screen.getByLabelText('标题'), '发布检查')
  await user.type(screen.getByLabelText('描述'), '发布前核对版本和说明')
  await user.type(
    screen.getByLabelText('内容（Markdown）'),
    '# 发布检查\n\n核对版本。',
  )
  await user.click(screen.getByRole('button', { name: '保存角色' }))
  const row = await screen.findByRole('article', { name: '发布检查' })
  await user.click(
    within(row).getByRole('button', { name: '查看角色 发布检查' }),
  )
  expect(await screen.findByText('核对版本。')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: '编辑角色' }))
  await user.clear(screen.getByLabelText('描述'))
  await user.type(screen.getByLabelText('描述'), '已更新')
  await user.click(screen.getByRole('button', { name: '保存角色' }))
  expect(
    within(await screen.findByRole('dialog')).getByText('已更新'),
  ).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: '删除角色' }))
  await user.click(
    within(await screen.findByRole('alertdialog')).getByRole('button', {
      name: '删除角色',
    }),
  )
  await waitFor(() =>
    expect(
      screen.queryByRole('article', { name: '发布检查' }),
    ).not.toBeInTheDocument(),
  )
})

it('imports a Markdown role and resolves a same-name conflict', async () => {
  const user = userEvent.setup()
  renderSettingsPage(<RolesPage />)
  await screen.findByRole('article', { name: '代码审查' })
  const fileInput = () => screen.getByLabelText('角色 Markdown 文件')
  const imported = new File(
    [
      `---
name: 发布检查
description: 发布前核对版本和说明
---

核对版本。`,
    ],
    'release.md',
    { type: 'text/markdown' },
  )
  await user.click(screen.getByRole('button', { name: '导入角色' }))
  await user.upload(fileInput(), imported)
  expect(await screen.findByText('发布检查')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: '确认导入' }))
  expect(
    await screen.findByRole('article', { name: '发布检查' }),
  ).toBeInTheDocument()

  const conflict = new File(
    [
      `---
name: 代码审查
description: 覆盖后的说明
---
覆盖后的内容`,
    ],
    'reviewer.md',
    { type: 'text/markdown' },
  )
  await user.click(screen.getByRole('button', { name: '导入角色' }))
  await user.upload(fileInput(), conflict)
  expect(await screen.findByText(/现有描述：/)).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: '覆盖' }))
  await user.click(screen.getByRole('button', { name: '确认导入' }))
  await waitFor(() =>
    expect(
      screen.getByRole('article', { name: '代码审查' }),
    ).toHaveTextContent('覆盖后的说明'),
  )
})
