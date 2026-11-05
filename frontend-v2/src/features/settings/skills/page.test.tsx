import { beforeEach, expect, it } from 'vitest'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import {
  installMarketplaceBackend,
  marketplaceEntry,
  marketplaceSelection,
  renderSettingsPage,
} from '../test-utils'
import type { Skill } from '@/lib/skills-api/generated'
import SkillsPage from './page'

const writing = marketplaceEntry('writing-kit', {
  kind: 'skill',
  title: '写作工具箱',
  description: '让团队文档保持清晰与一致。',
  readme: '# writing-guide\n\n先明确读者和目的，再组织内容。',
})

const serverSkill = (overrides: Partial<Skill> = {}): Skill => ({
  id: '00000000-0000-4000-8000-000000000801',
  name: 'review',
  description: '检查改动的正确性和测试覆盖。',
  content: '# 代码审查\n\n先看行为，再看实现。',
  version: 1,
  createdAt: '2026-11-02T08:00:00Z',
  updatedAt: '2026-11-02T08:00:00Z',
  ...overrides,
})

const skillFile = (
  name: string,
  description = '导入的技能',
  body = '# 导入说明\n\n按步骤执行。',
) =>
  new File(
    [`---\nname: ${name}\ndescription: ${description}\n---\n\n${body}`],
    'SKILL.md',
    { type: 'text/markdown' },
  )

beforeEach(() => localStorage.clear())

it('creates, edits, renders and deletes a server-backed skill', async () => {
  const backend = installMarketplaceBackend({
    catalog: [writing],
    skills: [serverSkill()],
  })
  const user = userEvent.setup()
  renderSettingsPage(<SkillsPage />)
  expect(await screen.findByRole('article', { name: 'review' })).toBeVisible()
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
  expect(backend.state.skills.map((skill) => skill.name)).not.toContain(
    'release-notes',
  )
})

it('imports one SKILL.md and asks before overwriting a conflict', async () => {
  const backend = installMarketplaceBackend({ catalog: [writing] })
  const user = userEvent.setup()
  renderSettingsPage(<SkillsPage />)
  await screen.findByRole('article', { name: 'writing-guide' })

  await user.click(screen.getByRole('button', { name: '导入技能' }))
  await user.upload(screen.getByLabelText(/选择 SKILL.md/), skillFile('review'))
  await user.click(screen.getByRole('button', { name: '导入' }))
  expect(await screen.findByRole('article', { name: 'review' })).toBeVisible()
  expect(backend.state.skills).toHaveLength(1)

  await user.click(screen.getByRole('button', { name: '导入技能' }))
  await user.upload(
    screen.getByLabelText(/选择 SKILL.md/),
    skillFile('review', '覆盖后的说明'),
  )
  await user.click(screen.getByRole('button', { name: '导入' }))
  expect(await screen.findByText('已有同名技能')).toBeVisible()
  await user.click(screen.getByRole('button', { name: '覆盖并导入' }))
  await waitFor(() => expect(backend.state.skills).toHaveLength(1))
  expect(backend.state.skills[0]).toMatchObject({
    name: 'review',
    description: '覆盖后的说明',
    version: 2,
  })
})

it('installs and removes the source plugin from the skill row and keeps the document readonly', async () => {
  const backend = installMarketplaceBackend({ catalog: [writing] })
  const user = userEvent.setup()
  renderSettingsPage(<SkillsPage />)
  const row = await screen.findByRole('article', { name: 'writing-guide' })
  expect(within(row).getByText('来源插件未安装')).toBeInTheDocument()
  await user.click(within(row).getByRole('button', { name: '安装技能' }))
  await waitFor(() => expect(within(row).getByText('可用')).toBeInTheDocument())
  expect(
    backend.state.selections.find(
      (selection) => selection.identifier === 'writing-kit',
    )?.desiredState,
  ).toBe('installed')
  await user.click(within(row).getByRole('button', { name: '移除技能' }))
  await user.click(
    within(await screen.findByRole('alertdialog')).getByRole('button', {
      name: '移除技能',
    }),
  )
  await waitFor(() =>
    expect(within(row).getByText('来源插件未安装')).toBeInTheDocument(),
  )
  await user.click(
    within(row).getByRole('button', { name: '查看技能 writing-guide' }),
  )
  expect(
    screen.queryByRole('button', { name: '编辑技能' }),
  ).not.toBeInTheDocument()
  expect(
    screen.getByRole('link', { name: /demo\/writing-kit/ }),
  ).toHaveAttribute('href', '/settings/plugins?plugin=demo%2Fwriting-kit')
})

it('treats a selected market skill as available without a project node receipt', async () => {
  installMarketplaceBackend({
    catalog: [writing],
    selections: [
      marketplaceSelection(writing, {
        observedState: 'pending',
        observedVersion: null,
      }),
    ],
  })
  renderSettingsPage(<SkillsPage />)
  const row = await screen.findByRole('article', { name: 'writing-guide' })
  expect(within(row).getByText('可用')).toBeInTheDocument()
  expect(within(row).queryByText('等待节点')).not.toBeInTheDocument()
  expect(within(row).getByRole('button', { name: '移除技能' })).toBeVisible()
})

it('shows a member the market state without market write controls', async () => {
  installMarketplaceBackend({
    role: 'member',
    catalog: [writing],
    selections: [marketplaceSelection(writing)],
  })
  renderSettingsPage(<SkillsPage />)
  const row = await screen.findByRole('article', { name: 'writing-guide' })
  expect(within(row).getByText('可用')).toBeInTheDocument()
  expect(
    within(row).queryByRole('button', { name: '移除技能' }),
  ).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: '新建技能' })).toBeVisible()
})
