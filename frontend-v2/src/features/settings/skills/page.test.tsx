import { beforeEach, expect, it } from 'vitest'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderSettingsPage } from '../test-utils'
import SkillsPage from './page'

beforeEach(() => localStorage.clear())
it('creates and edits a document, renders Markdown and deletes only after confirmation', async () => {
  const user = userEvent.setup()
  renderSettingsPage(<SkillsPage />)
  await screen.findByRole('article', { name: 'change-summary' })
  await user.click(screen.getByRole('button', { name: '新建技能' }))
  await user.type(screen.getByLabelText('技能名称'), 'release-notes')
  await user.type(screen.getByLabelText('简介'), '发布说明')
  await user.type(
    screen.getByLabelText('说明文档（Markdown）'),
    '# 发布清单\n\n检查兼容性。',
  )
  await user.click(screen.getByRole('button', { name: '保存技能' }))
  const row = await screen.findByRole('article', { name: 'release-notes' })
  await user.click(
    within(row).getByRole('button', { name: '查看技能 release-notes' }),
  )
  expect(
    await screen.findByRole('heading', { name: '发布清单' }),
  ).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: '编辑技能' }))
  await user.clear(screen.getByLabelText('简介'))
  await user.type(screen.getByLabelText('简介'), '已更新')
  await user.click(screen.getByRole('button', { name: '保存技能' }))
  await screen.findByRole('button', { name: '编辑技能' })
  await user.click(screen.getByRole('button', { name: '删除技能' }))
  await user.click(
    within(await screen.findByRole('alertdialog')).getByRole('button', {
      name: '删除技能',
    }),
  )
  await waitFor(() =>
    expect(
      screen.queryByRole('article', { name: 'release-notes' }),
    ).not.toBeInTheDocument(),
  )
})
it('shows plugin documents as readonly and links to their actual source package', async () => {
  const user = userEvent.setup()
  renderSettingsPage(<SkillsPage />)
  await user.click(
    await screen.findByRole('button', { name: '查看技能 writing-guide' }),
  )
  expect(
    screen.queryByRole('button', { name: '编辑技能' }),
  ).not.toBeInTheDocument()
  expect(
    screen.getByRole('link', { name: /demo\/writing-kit/ }),
  ).toHaveAttribute('href', '/settings/plugins?plugin=demo%2Fwriting-kit')
})
