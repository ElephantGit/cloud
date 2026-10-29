import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs'
import { useLoginProviders } from '@/features/auth/providers'
import { useMembers, useJoinRequests } from '@/features/members/api'
import { HuaweiAdd } from '@/features/members/directory-add'
import { MemberRoster } from '@/features/members/member-roster'
import { JoinLinks } from '@/features/members/join-links'
import { JoinRequests } from '@/features/members/join-requests'
import { normalizeSpaceRole } from '@/features/spaces/api'
import { useCurrentSpace } from '@/features/spaces/current-space'

/** Resolves space authority before mounting any administrator-only queries. */
export function MembersPage({ slug }: { slug: string }) {
  const { tenantId, space } = useCurrentSpace()
  const { data: providers } = useLoginProviders()
  if (!tenantId || space?.slug !== slug) return null
  return (
    <MemberScreen
      key={tenantId}
      tenantId={tenantId}
      canManage={normalizeSpaceRole(space.role) === 'admin'}
      directory={providers?.includes('huawei-idaas') ?? false}
    />
  )
}

function MemberScreen({
  tenantId,
  canManage,
  directory,
}: {
  tenantId: string
  canManage: boolean
  directory: boolean
}) {
  const { t } = useTranslation()
  const members = useMembers(tenantId)
  return (
    <div className="mx-auto w-full max-w-6xl space-y-6 p-4 md:p-6">
      <h1 className="text-xl font-semibold">{t('entry.title')}</h1>
      {canManage ? (
        <MemberAdministration tenantId={tenantId} directory={directory} members={members} />
      ) : (
        <RosterContent tenantId={tenantId} members={members} canManage={false} />
      )}
    </div>
  )
}

function RosterContent({
  tenantId,
  members,
  canManage,
}: {
  tenantId: string
  members: ReturnType<typeof useMembers>
  canManage: boolean
}) {
  const { t } = useTranslation()
  return (
    <section aria-label={t('entry.members')}>
      {members.isPending && <p role="status">{t('entry.loading')}</p>}
      {members.isError && (
        <p role="alert">
          {t('entry.loadFailed')}{' '}
          <Button variant="outline" onClick={() => void members.refetch()}>
            {t('entry.retry')}
          </Button>
        </p>
      )}
      {members.data && (
        <MemberRoster tenantId={tenantId} members={members.data.items} canManage={canManage} />
      )}
    </section>
  )
}

function MemberAdministration({
  tenantId,
  members,
  directory,
}: {
  tenantId: string
  members: ReturnType<typeof useMembers>
  directory: boolean
}) {
  const { t } = useTranslation()
  const [tab, setTab] = useState<'members' | 'requests' | 'links'>('members')
  const [linkFilter, setLinkFilter] = useState<string | undefined>()
  const requests = useJoinRequests(directory ? '' : tenantId)
  const pending = requests.data?.items.filter((row) => row.status === 'pending').length ?? 0
  return (
    <Tabs
      className="space-y-5"
      value={tab}
      onValueChange={(value) => {
        if (value === 'members' || value === 'requests' || value === 'links') {
          setTab(value)
          setLinkFilter(undefined)
        }
      }}
    >
      <TabsList variant="line" aria-label={t('entry.title')}>
        <TabsTrigger value="members">
          {t('entry.members')} ({members.data?.items.length ?? 0})
        </TabsTrigger>
        {!directory && (
          <TabsTrigger value="requests">
            {t('entry.requests')} ({pending})
          </TabsTrigger>
        )}
        {!directory && <TabsTrigger value="links">{t('entry.links')}</TabsTrigger>}
      </TabsList>
      <TabsContent value="members">
        {tab === 'members' && (
          <>
            {directory && <HuaweiAdd tenantId={tenantId} />}
            <RosterContent tenantId={tenantId} members={members} canManage />
          </>
        )}
      </TabsContent>
      <TabsContent value="requests">
        {tab === 'requests' && (
          <JoinRequests tenantId={tenantId} requests={requests} linkFilter={linkFilter} />
        )}
      </TabsContent>
      <TabsContent value="links">
        {tab === 'links' && (
          <JoinLinks
            tenantId={tenantId}
            onViewRequests={(id) => {
              setLinkFilter(id)
              setTab('requests')
            }}
          />
        )}
      </TabsContent>
    </Tabs>
  )
}
