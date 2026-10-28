import { screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { renderWithProviders } from '@/test/render'
import { SquadsPage } from './squads-page'

describe('Squad management entry', () => {
  it.each(['real-workspace', 'another-space'])(
    'explains unavailable Squads and links to the real roster in %s',
    (slug) => {
      renderWithProviders(<SquadsPage slug={slug} />)
      expect(screen.getByRole('heading', { name: '小队暂不可用' })).toBeInTheDocument()
      expect(screen.getByText(/通过邀请加入工作区的成员/)).toBeInTheDocument()
      expect(screen.getByRole('link', { name: '查看工作区成员' })).toHaveAttribute(
        'href',
        `/w/${slug}/settings/members`,
      )
      expect(screen.queryByText('平台组')).not.toBeInTheDocument()
      expect(screen.queryByText('产品组')).not.toBeInTheDocument()
    },
  )
})
