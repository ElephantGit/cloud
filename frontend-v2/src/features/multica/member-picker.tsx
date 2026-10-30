import { useState } from 'react'
import { Bot, ShieldCheck } from 'lucide-react'
import { useGetApiV1TenantsTidMembers } from '@/api/members/members'
import { useGetApiV1TenantsTidSpaces, useGetApiV1TenantsTidSpacesSpaceIdPlugins } from '@/api/spaces/spaces'
import type { MemberListItem, SpacePlugin } from '@/api/generated.schemas'
import { Button } from '@/components/ui/button'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import type { SquadDetail } from './api/generated'
import { agentCandidatesFor, candidatesFor, matchingMembers, matchingPlugins, pluginLabel } from './member-model'

/** Workspace roles as people read them; unknown roles stay as the server sent them. */
const ROLE_LABELS: Record<string, string> = { owner: '管理员', admin: '管理员', member: '成员' }

/**
 * Picks an existing workspace member or an installed Space Agent to add to one squad. Both lists
 * come from contract routes, so the picker can never offer an account outside this workspace or an
 * agent that is not installed in its space.
 */
export function MemberPicker({ squad, tenantId, adding, error, onAdd, onAddAgent, onClose }: {
  squad: SquadDetail
  tenantId: string
  /** A member is being added right now. */
  adding: boolean
  /** Error from the last add attempt. */
  error: string
  onAdd: (member: MemberListItem, role: string) => void
  onAddAgent: (plugin: SpacePlugin, role: string) => void
  onClose: () => void
}) {
  const roster = useGetApiV1TenantsTidMembers(tenantId, undefined, { query: { enabled: !!tenantId } })
  const spaces = useGetApiV1TenantsTidSpaces(tenantId, undefined, { query: { enabled: !!tenantId } })
  const spaceId = spaces.data?.items[0]?.id ?? ''
  const plugins = useGetApiV1TenantsTidSpacesSpaceIdPlugins(tenantId, spaceId, { query: { enabled: !!tenantId && !!spaceId } })
  const [query, setQuery] = useState('')
  const [role, setRole] = useState('')
  const candidates = matchingMembers(candidatesFor(squad, roster.data?.items ?? []), query)
  const agents = matchingPlugins(agentCandidatesFor(squad, plugins.data?.items ?? []), query)

  return <div className="mc-picker">
    <div className="mc-picker-scroll">
      <FieldGroup>
        <Field>
          <FieldLabel htmlFor="mc-member-search">搜索工作区成员</FieldLabel>
          <Input id="mc-member-search" value={query} onChange={event => setQuery(event.target.value)} placeholder="按姓名筛选" />
        </Field>
        <Field>
          <FieldLabel htmlFor="mc-member-role">职责（可选）</FieldLabel>
          <Input id="mc-member-role" value={role} onChange={event => setRole(event.target.value)} maxLength={500} placeholder="例如：负责调研" />
        </Field>
      </FieldGroup>

      <section className="mc-section" aria-label="可加入的工作区成员">
        <h3>工作区成员</h3>
        {roster.isLoading
          ? <p role="status" className="mc-empty-line">正在加载工作区成员…</p>
          : roster.isError
            ? <p role="alert" className="mc-empty-line">工作区成员加载失败，请关闭后重试。</p>
            : candidates.length === 0
              ? <p className="mc-empty-line">{query ? '没有匹配的成员。' : '工作区里没有可以加入的成员；先把人加入工作区。'}</p>
              : <ul className="mc-candidates">{candidates.map(member => <li key={member.id}>
                <span className="mc-member-name">{member.displayName}<em>{ROLE_LABELS[member.role] ?? member.role}</em></span>
                <Button type="button" variant="outline" size="sm" disabled={adding} onClick={() => onAdd(member, role)}>加入</Button>
              </li>)}</ul>}
      </section>

      <section className="mc-section" aria-label="可加入的空间 Agent">
        <h3>空间 Agent</h3>
        {spaces.isLoading || plugins.isLoading
          ? <p role="status" className="mc-empty-line">正在加载空间 Agent…</p>
          : spaces.isError || plugins.isError
            ? <p role="alert" className="mc-empty-line">空间 Agent 加载失败，请关闭后重试。</p>
            : agents.length === 0
              ? <p className="mc-empty-line">{query ? '没有匹配的 Agent。' : '空间里没有可以加入的 Agent；先在工作区空间安装 Agent。'}</p>
              : <ul className="mc-candidates">{agents.map(plugin => <li key={plugin.id}>
                <span className="mc-member-name">{pluginLabel(plugin)}<em>{plugin.observedVersion ? `已安装 ${plugin.observedVersion}` : '已安装'}</em></span>
                <Button type="button" variant="outline" size="sm" disabled={adding} onClick={() => onAddAgent(plugin, role)}>加入</Button>
              </li>)}</ul>}
      </section>

      <p className="mc-note"><Bot size={14} />只列出空间里已安装的 Agent；加入只建立本队编组，不启动 Agent 执行。</p>
      <p className="mc-note"><ShieldCheck size={14} />加入只建立本队编组；移出小队不影响工作区成员资格，也不授予任何运行时权限。</p>
      {error && <p role="alert" className="mc-error">{error}</p>}
    </div>

    <footer className="mc-detail-footer">
      <Button type="button" variant="outline" size="sm" onClick={onClose}>完成</Button>
    </footer>
  </div>
}
