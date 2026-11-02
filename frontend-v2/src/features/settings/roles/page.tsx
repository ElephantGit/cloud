import { useState } from 'react'
import { ArrowUpRight, Bot, Plus, Upload } from 'lucide-react'
import { toast } from 'sonner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
import { useSettingsData } from '../data/hooks'
import { deleteRole, saveRole } from '../data/resource-actions'
import type { Role, RoleDraft } from '../data/types'
import { ConfirmDialog } from '../components/confirm-dialog'
import { DetailPanel } from '../components/detail-panel'
import { MarkdownPreview } from '../components/markdown-preview'
import {
  DataState,
  NoResults,
  PageHeader,
  SearchInput,
} from '../components/page-parts'
import { RoleForm } from './role-form'
import { RoleImportDialog } from './import-dialog'
import { applyRoleImport } from './import'

type Panel = { kind: 'create' } | { kind: 'detail' | 'edit'; id: string } | null

export default function RolesPage() {
  const local = useSettingsData()
  const [search, setSearch] = useState('')
  const [panel, setPanel] = useState<Panel>(null)
  const [importing, setImporting] = useState(false)
  const [removing, setRemoving] = useState<Role>()
  const roles = local.query.data?.roles ?? []
  const filtered = roles.filter((role) =>
    `${role.name} ${role.description} ${
      role.content
    }`.toLowerCase().includes(search.trim().toLowerCase()),
  )
  const selected =
    panel && 'id' in panel
      ? roles.find((role) => role.id === panel.id)
      : undefined
  const open = (next: Panel) => {
    local.mutation.reset()
    setPanel(next)
  }
  const create = () => open({ kind: 'create' })
  const startImport = () => {
    local.mutation.reset()
    setImporting(true)
  }
  const save = (draft: RoleDraft, id?: string) =>
    local.save(
      (data) => saveRole(data, draft, id),
      '角色已保存',
      () => setPanel(id ? { kind: 'detail', id } : null),
    )
  return (
    <section>
      <PageHeader
        title="角色"
        description="管理可在工作流 Agent 节点中调用的角色；角色只保存在当前浏览器。"
        count={roles.length}
        actions={
          <>
            <Button
              variant="outline"
              disabled={!local.query.data || local.query.isError}
              onClick={startImport}
            >
              <Upload data-icon="inline-start" />
              导入角色
            </Button>
            <Button
              disabled={!local.query.data || local.query.isError}
              onClick={create}
            >
              <Plus data-icon="inline-start" />
              新建角色
            </Button>
          </>
        }
      />
      <DataState
        pending={local.query.isPending}
        error={local.query.error}
        retry={() => void local.query.refetch()}
      >
        <div className="settings-toolbar">
          <SearchInput label="搜索角色" value={search} onChange={setSearch} />
        </div>
        <div className="settings-resource-list">
          {filtered.map((role) => (
            <article
              key={role.id}
              aria-label={role.name}
              className="settings-resource-row"
            >
              <button
                type="button"
                className="settings-resource-main"
                aria-label={`查看角色 ${role.name}`}
                onClick={() => open({ kind: 'detail', id: role.id })}
              >
                <span className="settings-resource-icon">
                  <Bot className="size-5" />
                </span>
                <span className="min-w-0">
                  <span className="settings-resource-title">{role.name}</span>
                  <span className="settings-resource-description block">
                    {role.description || '暂无描述'}
                  </span>
                  <span className="settings-resource-meta">
                    <span>本地角色</span>
                    <span aria-hidden="true">·</span>
                    <span>{role.namespace}</span>
                  </span>
                </span>
              </button>
              <div className="settings-resource-actions">
                <Badge variant="secondary">可用</Badge>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => open({ kind: 'detail', id: role.id })}
                >
                  查看详情
                  <ArrowUpRight data-icon="inline-end" />
                </Button>
              </div>
            </article>
          ))}
        </div>
        {!filtered.length && (
          <NoResults
            filtered={roles.length > 0}
            onClear={() => setSearch('')}
          >
            <Button onClick={create}>新建角色</Button>
          </NoResults>
        )}
        <div className="settings-list-footer">
          <span>
            显示 {filtered.length} / {roles.length} 个角色
          </span>
          <span>工作流按角色标题引用；修改标题不会改写已有图</span>
        </div>
      </DataState>
      {panel?.kind === 'create' && (
        <DetailPanel
          title="新建角色"
          description="写清楚这个角色负责什么，以及它应该怎样工作。"
          onClose={() => setPanel(null)}
        >
          <RoleForm
            pending={local.mutation.isPending}
            error={local.mutation.error}
            onCancel={() => setPanel(null)}
            onSave={(draft) => save(draft)}
          />
        </DetailPanel>
      )}
      {selected && panel?.kind === 'edit' && (
        <DetailPanel
          title={`编辑 ${selected.name}`}
          description="修改只保存在当前浏览器。"
          onClose={() => setPanel(null)}
        >
          <RoleForm
            key={selected.id}
            role={selected}
            pending={local.mutation.isPending}
            error={local.mutation.error}
            onCancel={() => open({ kind: 'detail', id: selected.id })}
            onSave={(draft) => save(draft, selected.id)}
          />
        </DetailPanel>
      )}
      {selected && panel?.kind === 'detail' && (
        <DetailPanel
          title={selected.name}
          description={selected.description || '角色说明'}
          onClose={() => setPanel(null)}
        >
          <div className="flex flex-wrap items-center justify-between gap-3">
            <Badge variant="secondary">本地角色</Badge>
            <div className="flex gap-2">
              <Button
                variant="outline"
                onClick={() => open({ kind: 'edit', id: selected.id })}
              >
                编辑角色
              </Button>
              <Button
                variant="destructive"
                onClick={() => {
                  local.mutation.reset()
                  setRemoving(selected)
                }}
              >
                删除角色
              </Button>
            </div>
          </div>
          <dl className="settings-facts">
            <dt>命名空间</dt>
            <dd>{selected.namespace}</dd>
            <dt>引用方式</dt>
            <dd>工作流 Agent 节点按标题选择角色</dd>
          </dl>
          <Separator />
          <MarkdownPreview content={selected.content} />
        </DetailPanel>
      )}
      {importing && (
        <RoleImportDialog
          roles={roles}
          pending={local.mutation.isPending}
          error={local.mutation.error}
          onClose={() => setImporting(false)}
          onCommit={(preview, decision, done) => {
            if (decision === 'skip') {
              toast.success(`已跳过同名角色 ${preview.draft.name}`)
              done()
              return
            }
            local.save(
              (data) => applyRoleImport(data, preview, decision),
              '角色已导入',
              done,
            )
          }}
        />
      )}
      {removing && (
        <ConfirmDialog
          title={`删除“${removing.name}”？`}
          description="该角色将从工作流选择中移除；已有图里的标题引用不会被自动改写。此操作无法撤销。"
          confirmLabel="删除角色"
          pending={local.mutation.isPending}
          error={local.mutation.error}
          onClose={() => setRemoving(undefined)}
          onConfirm={() =>
            local.save(
              (data) => deleteRole(data, removing.id),
              '角色已删除',
              () => {
                setRemoving(undefined)
                setPanel(null)
              },
            )
          }
        />
      )}
    </section>
  )
}
