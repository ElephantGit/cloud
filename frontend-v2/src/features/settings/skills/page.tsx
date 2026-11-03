import { useState } from 'react'
import { Link } from 'react-router'
import { toast } from 'sonner'
import {
  ArrowUpRight,
  Download,
  FileText,
  Plus,
  Sparkles,
  Upload,
} from 'lucide-react'
import type {
  PluginCatalogEntry,
  SpacePlugin,
} from '@/api/generated.schemas'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
import {
  pluginInstalled,
  pluginKey,
  usePluginMarketplace,
} from '../data/marketplace'
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
import { MarkdownPreview } from '../components/markdown-preview'
import type { Skill as ServerSkill, SkillInput } from '@/lib/skills-api/generated'
import { ImportSkillDialog } from './import-skill-dialog'
import { SkillForm } from './skill-form'
import { useUserSkills } from './use-skills'

type Panel = { kind: 'create' } | { kind: 'detail' | 'edit'; id: string } | null

type SkillSource =
  | { kind: 'local' }
  | { kind: 'plugin'; pluginId: string }

type SkillRow = {
  id: string
  namespace: string
  name: string
  description: string
  content: string
  source: SkillSource
  availability: 'available' | 'unavailable'
  updatedAt?: string
  local?: ServerSkill
  entry?: PluginCatalogEntry
  selection?: SpacePlugin
}

type Removing =
  | { kind: 'local'; skill: SkillRow }
  | { kind: 'plugin'; skill: SkillRow }

function skillStatus(skill: SkillRow, detail = false) {
  if (skill.availability === 'available') return '可用'
  if (skill.selection?.observedState === 'removing') return '移除中'
  if (skill.selection?.observedState === 'failed')
    return skill.selection.desiredState === 'removed' ? '移除失败' : '安装失败'
  if (skill.selection?.desiredState === 'installed') return '等待节点'
  return detail ? '不可用，来源插件未安装' : '来源插件未安装'
}

function pluginSkillName(entry: PluginCatalogEntry): string {
  const heading = entry.readme?.match(/^#\s+(.+)$/m)?.[1]?.trim()
  return heading || entry.identifier
}

function localSkillRows(skills: ServerSkill[]): SkillRow[] {
  return skills.map((skill) => ({
    ...skill,
    namespace: 'local',
    source: { kind: 'local' },
    availability: 'available',
    local: skill,
  }))
}

function pluginSkillRows(
  catalog: PluginCatalogEntry[],
  selection: (entry: PluginCatalogEntry) => SpacePlugin | undefined,
): SkillRow[] {
  return catalog
    .filter((entry) => entry.kind === 'skill')
    .map((entry) => {
      const chosen = selection(entry)
      return {
        id: `plugin-skill-${pluginKey(entry)}`,
        namespace: entry.sourceNamespace,
        name: pluginSkillName(entry),
        description: entry.description,
        content: entry.readme ?? '',
        source: { kind: 'plugin', pluginId: pluginKey(entry) },
        availability: pluginInstalled(chosen) ? 'available' : 'unavailable',
        entry,
        selection: chosen,
      }
    })
}

const updatedLabel = (value?: string) => {
  if (!value) return '未知'
  const date = new Date(value)
  return Number.isNaN(date.valueOf())
    ? '未知'
    : new Intl.DateTimeFormat('zh-CN', {
        year: 'numeric',
        month: 'short',
        day: 'numeric',
        hour: '2-digit',
        minute: '2-digit',
      }).format(date)
}

export default function SkillsPage() {
  const local = useUserSkills()
  const market = usePluginMarketplace()
  const [search, setSearch] = useState('')
  const [source, setSource] = useState('all')
  const [status, setStatus] = useState('all')
  const [order, setOrder] = useState('asc')
  const [panel, setPanel] = useState<Panel>(null)
  const [removing, setRemoving] = useState<Removing>()
  const [importing, setImporting] = useState(false)
  const skills: SkillRow[] = [
    ...localSkillRows(local.skills),
    ...pluginSkillRows(market.catalog, market.selection),
  ]
  const selected =
    panel && 'id' in panel
      ? skills.find((skill) => skill.id === panel.id)
      : undefined
  const filtered = skills
    .filter(
      (skill) =>
        `${skill.name} ${skill.description} ${skill.namespace}`
          .toLowerCase()
          .includes(search.trim().toLowerCase()) &&
        (source === 'all' || skill.source.kind === source) &&
        (status === 'all' || skill.availability === status),
    )
    .sort(
      (a, b) =>
        (order === 'asc' ? 1 : -1) * a.name.localeCompare(b.name, 'zh-CN'),
    )
  const localSkills = skills.filter((skill) => skill.source.kind === 'local')
  const clear = () => {
    setSearch('')
    setSource('all')
    setStatus('all')
  }
  const open = (next: Panel) => {
    local.resetErrors()
    market.resetErrors()
    setPanel(next)
  }
  const create = () => open({ kind: 'create' })
  const install = (skill: SkillRow) => {
    if (!skill.entry) return
    market.resetErrors()
    void market
      .install(skill.entry)
      .then(() => toast.success(`已选择技能 ${skill.name}`))
      .catch(() => undefined)
  }
  const requestPluginRemove = (skill: SkillRow) => {
    market.resetErrors()
    if (skill.selection) setRemoving({ kind: 'plugin', skill })
  }
  const confirmPluginRemove = () => {
    if (removing?.kind !== 'plugin' || !removing.skill.selection) return
    void market
      .remove(removing.skill.selection)
      .then(() => {
        toast.success(`已提交移除技能 ${removing.skill.name}`)
        setRemoving(undefined)
      })
      .catch(() => undefined)
  }
  const save = async (
    input: SkillInput,
    id: string | undefined,
    done: () => void,
  ) => {
    if (!local.tenantId) return
    local.resetErrors()
    try {
      if (id)
        await local.update({ tid: local.tenantId, sid: id, data: input })
      else await local.create({ tid: local.tenantId, data: input })
      toast.success(id ? `已更新技能 ${input.name}` : `已创建技能 ${input.name}`)
      done()
    } catch {
      // The hook exposes the localized mutation error beside the form.
    }
  }
  const confirmLocalRemove = async () => {
    if (removing?.kind !== 'local' || !removing.skill.local) return
    try {
      await local.remove({
        tid: local.tenantId,
        sid: removing.skill.local.id,
      })
      toast.success(`已删除技能 ${removing.skill.name}`)
      setRemoving(undefined)
      setPanel(null)
    } catch {
      // ConfirmDialog renders the localized mutation error.
    }
  }
  function pluginActions(skill: SkillRow) {
    if (!skill.entry || !market.canManage) return null
    if (skill.selection?.observedState === 'removing')
      return (
        <span className="text-xs text-muted-foreground">
          正在等待项目节点确认
        </span>
      )
    if (
      skill.selection?.desiredState === 'removed' &&
      skill.selection.observedState === 'failed'
    )
      return (
        <Button
          size="sm"
          variant="outline"
          disabled={market.mutating}
          onClick={() => requestPluginRemove(skill)}
        >
          重试移除
        </Button>
      )
    return skill.selection?.desiredState === 'installed' ? (
      <Button
        variant="outline"
        size="sm"
        disabled={market.mutating}
        onClick={() => requestPluginRemove(skill)}
      >
        移除技能
      </Button>
    ) : (
      <Button
        variant="secondary"
        size="sm"
        disabled={market.mutating}
        onClick={() => install(skill)}
      >
        <Download data-icon="inline-start" />
        安装技能
      </Button>
    )
  }
  return (
    <section>
      <PageHeader
        title="Skills"
        description="管理当前工作区的可复用技能。导入或新建后，新开启的 Agent 会话会在项目中自动发现它们。"
        count={skills.length}
        actions={
          <>
            <Button
              variant="outline"
              disabled={!local.tenantId || local.loading}
              onClick={() => {
                local.resetErrors()
                setImporting(true)
              }}
            >
              <Upload data-icon="inline-start" />
              导入技能
            </Button>
            <Button
              disabled={!local.tenantId || local.loading}
              onClick={create}
            >
              <Plus data-icon="inline-start" />
              新建技能
            </Button>
          </>
        }
      />
      <DataState
        pending={local.loading || market.pending}
        error={local.error ?? market.error}
        retry={() => {
          local.refetch()
          void market.refetch()
        }}
      >
        <div className="settings-toolbar">
          {market.spaces.length > 1 && (
            <Filter
              label="空间"
              value={market.spaceId}
              onChange={(id) => {
                market.chooseSpace(id)
                setPanel(null)
                setRemoving(undefined)
              }}
              options={market.spaces.map((s) => [s.id, s.name] as const)}
            />
          )}
          <SearchInput label="搜索技能" value={search} onChange={setSearch} />
          <Filter
            label="技能来源"
            value={source}
            onChange={setSource}
            options={[
              ['all', '全部来源'],
              ['local', '用户技能'],
              ['plugin', '市场技能'],
            ]}
          />
          <Filter
            label="技能状态"
            value={status}
            onChange={setStatus}
            options={[
              ['all', '全部状态'],
              ['available', '可用'],
              ['unavailable', '不可用'],
            ]}
          />
          <Filter
            label="技能排序"
            value={order}
            onChange={setOrder}
            options={[
              ['asc', '名称 A–Z'],
              ['desc', '名称 Z–A'],
            ]}
          />
        </div>
        <ActionError error={local.mutationError ?? market.mutationError} />
        <div className="settings-resource-list">
          {filtered.map((skill) => (
            <article
              key={skill.id}
              aria-label={skill.name}
              className="settings-resource-row"
            >
              <button
                type="button"
                className="settings-resource-main"
                aria-label={`查看技能 ${skill.name}`}
                onClick={() => open({ kind: 'detail', id: skill.id })}
              >
                <span className="settings-resource-icon">
                  {skill.source.kind === 'local' ? (
                    <FileText className="size-5" />
                  ) : (
                    <Sparkles className="size-5" />
                  )}
                </span>
                <span className="min-w-0">
                  <span className="settings-resource-title">{skill.name}</span>
                  <span className="settings-resource-description block">
                    {skill.description || '暂无简介'}
                  </span>
                  <span className="settings-resource-meta">
                    <span>
                      {skill.source.kind === 'local'
                        ? '用户技能 · 服务端'
                        : '市场技能 · 只读'}
                    </span>
                    <span aria-hidden="true">·</span>
                    <span>{skill.namespace}</span>
                  </span>
                </span>
              </button>
              <div className="settings-resource-actions">
                <Badge
                  variant={
                    skill.availability === 'available' ? 'secondary' : 'outline'
                  }
                >
                  {skillStatus(skill)}
                </Badge>
                {pluginActions(skill)}
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => open({ kind: 'detail', id: skill.id })}
                >
                  查看详情
                  <ArrowUpRight data-icon="inline-end" />
                </Button>
              </div>
            </article>
          ))}
        </div>
        {!filtered.length && (
          <NoResults filtered={skills.length > 0} onClear={clear}>
            <div className="flex flex-wrap justify-center gap-2">
              <Button
                variant="outline"
                onClick={() => {
                  local.resetErrors()
                  setImporting(true)
                }}
              >
                导入 SKILL.md
              </Button>
              <Button onClick={create}>新建技能</Button>
            </div>
          </NoResults>
        )}
        <div className="settings-list-footer">
          <span>
            显示 {filtered.length} / {skills.length} 个技能 · 共{' '}
            {localSkills.length} 个用户技能
          </span>
          <span>用户技能保存在服务端；市场技能跟随插件安装</span>
        </div>
      </DataState>
      {panel?.kind === 'create' && (
        <DetailPanel
          title="新建技能"
          description="写一份可复用的 Agent 技能说明。"
          onClose={() => setPanel(null)}
        >
          <SkillForm
            pending={local.savePending}
            error={local.mutationError}
            onCancel={() => setPanel(null)}
            onSave={(draft) =>
              void save(draft, undefined, () => setPanel(null))
            }
          />
        </DetailPanel>
      )}
      {selected?.local && panel?.kind === 'edit' && (
        <DetailPanel
          title={`编辑 ${selected.name}`}
          description="保存后会更新服务端目录；已有会话保留启动时的技能副本。"
          onClose={() => setPanel(null)}
        >
          <SkillForm
            key={selected.id}
            skill={selected.local}
            pending={local.savePending}
            error={local.mutationError}
            onCancel={() => open({ kind: 'detail', id: selected.id })}
            onSave={(draft) =>
              void save(draft, selected.local?.id, () =>
                setPanel({ kind: 'detail', id: selected.id }),
              )
            }
          />
        </DetailPanel>
      )}
      {selected && panel?.kind === 'detail' && (
        <DetailPanel
          title={selected.name}
          description={selected.description || '技能说明文档'}
          onClose={() => setPanel(null)}
        >
          <div className="flex flex-wrap items-center justify-between gap-3">
            <Badge variant="secondary">
              {selected.source.kind === 'local'
                ? '用户技能 · 服务端'
                : '市场技能 · 只读'}
            </Badge>
            {selected.local ? (
              <div className="flex gap-2">
                <Button
                  variant="outline"
                  onClick={() => open({ kind: 'edit', id: selected.id })}
                >
                  编辑技能
                </Button>
                <Button
                  variant="destructive"
                  onClick={() => {
                    local.resetErrors()
                    setRemoving({ kind: 'local', skill: selected })
                  }}
                >
                  删除技能
                </Button>
              </div>
            ) : (
              pluginActions(selected)
            )}
          </div>
          <dl className="settings-facts">
            <dt>命名空间</dt>
            <dd>{selected.namespace}</dd>
            <dt>可用状态</dt>
            <dd>{skillStatus(selected, true)}</dd>
            <dt>来源</dt>
            <dd>
              {selected.source.kind === 'local' ? (
                '当前登录用户的技能库'
              ) : (
                <Link
                  className="underline underline-offset-4"
                  to={`/settings/plugins?plugin=${encodeURIComponent(selected.source.pluginId)}`}
                >
                  {selected.source.pluginId} ↗
                </Link>
              )}
            </dd>
            {selected.local && (
              <>
                <dt>最近更新</dt>
                <dd>{updatedLabel(selected.updatedAt)}</dd>
                <dt>目录版本</dt>
                <dd>v{selected.local.version}</dd>
              </>
            )}
          </dl>
          {selected.availability === 'unavailable' && (
            <p className="rounded-lg bg-muted p-3 text-sm text-muted-foreground">
              说明仍可预览。安装来源插件后，此技能会恢复可用。
            </p>
          )}
          <p className="rounded-lg bg-muted/60 p-3 text-xs leading-relaxed text-muted-foreground">
            新建的 Agent 会话会读取 SKILL.md；编辑或删除不会回写已经开始运行的会话。
          </p>
          <Separator />
          <MarkdownPreview content={selected.content} />
        </DetailPanel>
      )}
      {importing && (
        <ImportSkillDialog
          onClose={() => setImporting(false)}
          onImport={async (input) => {
            local.resetErrors()
            return local.importFile({ tid: local.tenantId, data: input })
          }}
          onImported={(name, overwritten) => {
            toast.success(
              overwritten ? `已覆盖技能 ${name}` : `已导入技能 ${name}`,
            )
            setImporting(false)
          }}
        />
      )}
      {removing?.kind === 'local' && (
        <ConfirmDialog
          title={`删除 ${removing.skill.name}？`}
          description="将从服务端技能库删除这条技能。已经开始的 Agent 会话仍保留启动时的副本。"
          confirmLabel="删除技能"
          pending={local.deletePending}
          error={local.mutationError}
          onClose={() => setRemoving(undefined)}
          onConfirm={() => void confirmLocalRemove()}
        />
      )}
      {removing?.kind === 'plugin' && (
        <ConfirmDialog
          title={`移除技能 ${removing.skill.name}？`}
          description="将从空间卸载来源技能包。技能说明仍可预览，但会显示为不可用；重新安装可恢复。"
          confirmLabel="移除技能"
          pending={market.mutating}
          error={market.mutationError}
          onClose={() => setRemoving(undefined)}
          onConfirm={confirmPluginRemove}
        />
      )}
    </section>
  )
}
