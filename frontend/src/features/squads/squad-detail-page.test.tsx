import { screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { renderAtRoute } from '@/test/render'
import { db } from '@/mocks/data/store'
import { SquadDetailPage } from './squad-detail-page'

describe('Squad detail compatibility entry', () => {
  it.each(['squad-platform', 'unknown-squad'])(
    'opens %s without showing demo members or projects',
    (squadId) => {
      renderAtRoute(
        '/w/:workspaceSlug/squads/:squadId',
        <SquadDetailPage slug="invited-space" />,
        `/w/invited-space/squads/${squadId}`,
      )
      expect(screen.getByRole('link', { name: '查看工作区成员' })).toHaveAttribute(
        'href',
        '/w/invited-space/settings/members',
      )
      expect(screen.queryByRole('heading', { name: '成员' })).not.toBeInTheDocument()
      for (const record of [...db.squads, ...db.agents]) {
        expect(screen.queryByText(record.name)).not.toBeInTheDocument()
      }
      for (const project of db.projects) {
        expect(screen.queryByText(project.title)).not.toBeInTheDocument()
      }
    },
  )
})
