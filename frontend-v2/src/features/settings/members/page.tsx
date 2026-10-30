import { useState } from 'react'
import { useSearchParams } from 'react-router'
import type { MemberListItem } from '@/api/generated.schemas'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { useSettingsData } from '../data/hooks'
import { updateMember } from '../data/member-actions'
import {
  ActionError,
  DataState,
  Filter,
  NoResults,
  PageHeader,
  SearchInput,
} from '../components/page-parts'
import { DetailPanel } from '../components/detail-panel'
import { ConfirmDialog } from '../components/confirm-dialog'
import { MemberRoster } from './member-roster'
import { JoinManagement } from './join-management'

const methods: Record<string, string> = {
  workspace_created: '创建工作空间',
  invitation: '邀请加入',
  application: '申请批准',
  directory: '人员目录',
}
export default function MembersPage() {
  const { query, mutation, save, user, canManage } = useSettingsData()
  const [params, setParams] = useSearchParams()
  const [view, setView] = useState('members')
  const [search, setSearch] = useState('')
  const [role, setRole] = useState('all')
  const [status, setStatus] = useState('all')
  const [removing, setRemoving] = useState<MemberListItem>()
  const [historyMember, setHistoryMember] = useState<MemberListItem>()
  const members = query.data?.members ?? []
  const currentView = !canManage
    ? 'members'
    : params.has('preview')
      ? 'links'
      : view
  const filtered = members.filter(
    (m) =>
      m.displayName.toLowerCase().includes(search.trim().toLowerCase()) &&
      (role === 'all' || m.role === role) &&
      (status === 'all' || m.status === status),
  )
  const clear = () => {
    setSearch('')
    setRole('all')
    setStatus('all')
  }
  return (
    <section>
      <PageHeader
        title="成员"
        description="查看工作空间成员，管理角色与加入方式。"
        count={members.length}
      />
      <DataState
        pending={query.isPending}
        error={query.error}
        retry={() => void query.refetch()}
      >
        {canManage ? (
          <ToggleGroup
            aria-label="成员管理视图"
            value={[currentView]}
            onValueChange={(values) => {
              const next = values[0]
              if (typeof next === 'string') {
                setView(next)
                if (params.has('preview')) setParams({})
              }
            }}
            variant="outline"
            size="sm"
            className="mb-6 max-w-full flex-wrap"
          >
            <ToggleGroupItem value="members">成员列表</ToggleGroupItem>
            <ToggleGroupItem value="requests">
              加入申请 (
              {query.data?.requests.filter((r) => r.status === 'pending')
                .length ?? 0}
              )
            </ToggleGroupItem>
            <ToggleGroupItem value="links">邀请与链接</ToggleGroupItem>
          </ToggleGroup>
        ) : (
          <p className="mb-5 text-sm text-muted-foreground">
            当前演示身份为只读成员，管理操作仅向管理员开放。
          </p>
        )}
        {currentView === 'members' ? (
          <>
            <div className="settings-toolbar">
              <SearchInput
                label="搜索成员"
                value={search}
                onChange={setSearch}
              />
              <Filter
                label="成员角色"
                value={role}
                onChange={setRole}
                options={[
                  ['all', '全部角色'],
                  ['admin', '管理员'],
                  ['member', '成员'],
                ]}
              />
              <Filter
                label="成员状态"
                value={status}
                onChange={setStatus}
                options={[
                  ['all', '全部状态'],
                  ['active', '有效'],
                  ['disabled', '已停用'],
                ]}
              />
            </div>
            <ActionError error={mutation.error} />
            {filtered.length ? (
              <MemberRoster
                members={filtered}
                currentUserId={user.id}
                canManage={canManage}
                pending={mutation.isPending}
                onRole={(member, nextRole) =>
                  save(
                    (data) => updateMember(data, user, member.userId, nextRole),
                    '演示成员角色已更新',
                  )
                }
                onRemove={(member) => {
                  mutation.reset()
                  setRemoving(member)
                }}
                onHistory={setHistoryMember}
              />
            ) : (
              <NoResults filtered={members.length > 0} onClear={clear} />
            )}
            <div className="settings-list-footer">
              <span>{filtered.length} 名成员</span>
              <span>停用保留记录，重新加入需审批</span>
            </div>
          </>
        ) : (
          <JoinManagement
            view={currentView === 'links' ? 'links' : 'requests'}
          />
        )}
      </DataState>
      {historyMember && canManage && (
        <DetailPanel
          title={`${historyMember.displayName}的加入历史`}
          description="保留每次加入方式与审批来源；停用不会删除历史。"
          onClose={() => setHistoryMember(undefined)}
        >
          {query.data?.history
            .filter((h) => h.userId === historyMember.userId)
            .map((history) => (
              <div
                key={history.id}
                className="flex flex-col gap-2 border-l-2 pl-4"
              >
                <h3 className="text-sm font-medium">
                  {methods[history.method] ?? history.method}
                </h3>
                <p className="text-sm text-muted-foreground">
                  {history.actorDisplayName} ·{' '}
                  {new Date(history.joinedAt).toLocaleString('zh-CN')}
                </p>
                <p className="text-xs text-muted-foreground">
                  {history.invitationNote ?? history.linkNote ?? '演示工作空间'}
                </p>
              </div>
            ))}
        </DetailPanel>
      )}
      {removing && (
        <ConfirmDialog
          title={`停用 ${removing.displayName}？`}
          description="保留演示成员和加入历史。若要重新加入，需要经过新一轮申请审批；不会影响真实成员。"
          confirmLabel="停用成员"
          pending={mutation.isPending}
          error={mutation.error}
          onClose={() => setRemoving(undefined)}
          onConfirm={() =>
            save(
              (data) => updateMember(data, user, removing.userId, 'disabled'),
              '演示成员已停用',
              () => setRemoving(undefined),
            )
          }
        />
      )}
    </section>
  )
}
