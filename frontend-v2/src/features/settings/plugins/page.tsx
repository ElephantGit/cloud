import { useState } from 'react'
import { useSearchParams } from 'react-router'
import { Blocks, BookOpen, Download, ExternalLink, Puzzle } from 'lucide-react'
import type { PluginCatalogEntry, SpacePlugin } from '@/api/generated.schemas'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
import { useSettingsData } from '../data/hooks'
import { pluginKey } from '../data/types'
import { setPluginInstalled } from '../data/resource-actions'
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

const kindLabels: Record<string, string> = {
  mcp: 'MCP',
  skill: '技能包',
  webview: 'WebView',
  workflow: '工作流',
  hook: '钩子',
  workbench: '工作台',
  pack: '合集',
}
const stateLabels: Record<string, string> = {
  installed: '已安装',
  failed: '安装失败',
  pending: '等待安装',
  installing: '安装中',
  removing: '移除中',
  removed: '未安装',
}
function installed(selection?: SpacePlugin) {
  return (
    selection?.desiredState === 'installed' &&
    selection.observedState === 'installed'
  )
}
function StateBadge({ selection }: { selection?: SpacePlugin }) {
  const state = selection?.observedState ?? 'removed'
  return (
    <Badge
      variant={
        state === 'failed'
          ? 'destructive'
          : state === 'installed'
            ? 'secondary'
            : 'outline'
      }
    >
      {stateLabels[state] ?? state}
    </Badge>
  )
}

export default function PluginsPage() {
  const { query, mutation, save, user, canManage } = useSettingsData()
  const [search, setSearch] = useState('')
  const [kind, setKind] = useState('all')
  const [status, setStatus] = useState('all')
  const [order, setOrder] = useState('asc')
  const [removing, setRemoving] = useState<PluginCatalogEntry>()
  const [params, setParams] = useSearchParams()
  const data = query.data
  const selection = (key: string) =>
    data?.installations.find((p) => pluginKey(p) === key)
  const selected = data?.catalog.find(
    (p) => pluginKey(p) === params.get('plugin') && p.kind !== 'agent',
  )
  const catalog = (data?.catalog ?? []).filter((p) => p.kind !== 'agent')
  const filtered = catalog
    .filter((p) => {
      const chosen = selection(pluginKey(p))
      return (
        `${p.title} ${p.description} ${pluginKey(p)}`
          .toLowerCase()
          .includes(search.trim().toLowerCase()) &&
        (kind === 'all' || p.kind === kind) &&
        (status === 'all' ||
          (status === 'installed'
            ? installed(chosen)
            : status === 'failed'
              ? chosen?.observedState === 'failed'
              : !chosen || chosen.observedState === 'removed'))
      )
    })
    .sort(
      (a, b) =>
        (order === 'asc' ? 1 : -1) * a.title.localeCompare(b.title, 'zh-CN'),
    )
  const clear = () => {
    setSearch('')
    setKind('all')
    setStatus('all')
  }
  const open = (entry: PluginCatalogEntry) => {
    mutation.reset()
    setParams({ plugin: pluginKey(entry) })
  }
  const install = (entry: PluginCatalogEntry) =>
    save(
      (draft) => setPluginInstalled(draft, user.id, pluginKey(entry), true),
      '已在演示中安装插件',
    )
  const remove = (entry: PluginCatalogEntry) => {
    mutation.reset()
    setRemoving(entry)
  }
  function actions(entry: PluginCatalogEntry) {
    const chosen = selection(pluginKey(entry))
    if (entry.kind === 'pack')
      return (
        <span className="text-xs text-muted-foreground">合集安装尚未支持</span>
      )
    const busy =
      mutation.isPending ||
      chosen?.observedState === 'installing' ||
      chosen?.observedState === 'removing'
    return installed(chosen) ? (
      <Button
        size="sm"
        variant="outline"
        disabled={!canManage || busy}
        onClick={() => remove(entry)}
      >
        模拟移除
      </Button>
    ) : (
      <Button
        size="sm"
        variant={chosen?.observedState === 'failed' ? 'outline' : 'secondary'}
        disabled={!canManage || busy}
        onClick={() => install(entry)}
      >
        <Download data-icon="inline-start" />
        {chosen?.observedState === 'failed' ? '重试演示安装' : '模拟安装'}
      </Button>
    )
  }
  return (
    <section>
      <PageHeader
        title="Plugins"
        description="浏览插件，管理演示工作空间中的安装选择与版本。"
        count={catalog.length}
      />
      <DataState
        pending={query.isPending}
        error={query.error}
        retry={() => void query.refetch()}
      >
        {!canManage && data && (
          <p className="text-sm text-muted-foreground">
            当前演示成员为只读角色；安装与移除需要管理员权限。
          </p>
        )}
        <div className="settings-toolbar">
          <SearchInput label="搜索插件" value={search} onChange={setSearch} />
          <Filter
            label="插件类型"
            value={kind}
            onChange={setKind}
            options={[
              ['all', '全部类型'],
              ...[...new Set(catalog.map((p) => p.kind))].map(
                (k) => [k, kindLabels[k] ?? k] as const,
              ),
            ]}
          />
          <Filter
            label="安装状态"
            value={status}
            onChange={setStatus}
            options={[
              ['all', '全部状态'],
              ['installed', '已安装'],
              ['available', '未安装'],
              ['failed', '安装失败'],
            ]}
          />
          <Filter
            label="插件排序"
            value={order}
            onChange={setOrder}
            options={[
              ['asc', '名称 A–Z'],
              ['desc', '名称 Z–A'],
            ]}
          />
        </div>
        <ActionError error={mutation.error} />
        <div className="settings-resource-list">
          {filtered.map((entry) => (
            <article
              key={pluginKey(entry)}
              aria-label={entry.title}
              className="settings-resource-row"
            >
              <button
                type="button"
                className="settings-resource-main"
                onClick={() => open(entry)}
                aria-label={`查看插件 ${entry.title}`}
              >
                <span className="settings-resource-icon">
                  {entry.kind === 'skill' ? (
                    <BookOpen className="size-5" />
                  ) : entry.kind === 'pack' ? (
                    <Blocks className="size-5" />
                  ) : (
                    <Puzzle className="size-5" />
                  )}
                </span>
                <span className="min-w-0">
                  <span className="settings-resource-title">{entry.title}</span>
                  <span className="settings-resource-description block">
                    {entry.description}
                  </span>
                  <span className="settings-resource-meta">
                    <span>{kindLabels[entry.kind]}</span>
                    <span aria-hidden="true">·</span>
                    <span>演示目录</span>
                    <span aria-hidden="true">·</span>
                    <span>v{entry.version}</span>
                  </span>
                </span>
              </button>
              <div className="settings-resource-actions">
                <StateBadge selection={selection(pluginKey(entry))} />
                {actions(entry)}
              </div>
            </article>
          ))}
        </div>
        {!filtered.length && (
          <NoResults filtered={catalog.length > 0} onClear={clear} />
        )}
        <div className="settings-list-footer">
          <span>
            显示 {filtered.length} / {catalog.length} 个插件 · 共{' '}
            {data?.installations.filter(installed).length ?? 0} 个已安装
          </span>
          <span>安装与移除均为本机演示</span>
        </div>
      </DataState>
      {params.has('plugin') && data && !selected && (
        <p role="alert" className="mt-4 text-sm text-muted-foreground">
          找不到该插件。
          <Button variant="link" onClick={() => setParams({})}>
            返回全部插件
          </Button>
        </p>
      )}
      {selected && (
        <DetailPanel
          title={selected.title}
          description={selected.description}
          onClose={() => setParams({}, { replace: true })}
        >
          <div className="flex flex-wrap items-center justify-between gap-3">
            <StateBadge selection={selection(pluginKey(selected))} />
            {actions(selected)}
          </div>
          <ActionError error={mutation.error} />
          <dl className="settings-facts">
            <dt>插件标识</dt>
            <dd>{pluginKey(selected)}</dd>
            <dt>类型</dt>
            <dd>{kindLabels[selected.kind]}</dd>
            <dt>目录版本</dt>
            <dd>{selected.version}</dd>
            <dt>安装选择</dt>
            <dd>
              {selection(pluginKey(selected))?.desiredState === 'installed'
                ? '安装 ' + selection(pluginKey(selected))?.desiredVersion
                : '未选择'}
            </dd>
            <dt>安装结果</dt>
            <dd>
              {selection(pluginKey(selected))?.observedVersion ?? '尚未安装'}
            </dd>
            <dt>来源</dt>
            <dd>演示目录 · {selected.sourceNamespace}</dd>
            <dt>许可证</dt>
            <dd>{selected.license ?? '未提供'}</dd>
          </dl>
          {selection(pluginKey(selected))?.installError && (
            <p role="alert" className="text-sm text-destructive">
              {selection(pluginKey(selected))?.installError}
            </p>
          )}
          <p className="rounded-lg bg-muted p-3 text-xs leading-relaxed text-muted-foreground">
            此处只演示安装选择。插件运行、启停与配置能力尚未接入。
          </p>
          <Separator />
          <MarkdownPreview content={selected.readme ?? ''} />
          {selected.homepage?.startsWith('https://') && (
            <a
              href={selected.homepage}
              target="_blank"
              rel="noreferrer"
              className="flex items-center gap-2 text-sm underline"
            >
              插件主页
              <ExternalLink className="size-3" />
            </a>
          )}
        </DetailPanel>
      )}
      {removing && (
        <ConfirmDialog
          title={`移除 ${removing.title}？`}
          description={
            removing.kind === 'skill'
              ? '将移除演示安装，其提供的技能会变为不可用。自建技能不受影响，重新安装可恢复。'
              : '只移除此浏览器中的演示安装，不会卸载任何真实软件。'
          }
          confirmLabel="模拟移除"
          pending={mutation.isPending}
          error={mutation.error}
          onClose={() => setRemoving(undefined)}
          onConfirm={() =>
            save(
              (draft) =>
                setPluginInstalled(draft, user.id, pluginKey(removing), false),
              '已在演示中移除插件',
              () => setRemoving(undefined),
            )
          }
        />
      )}
    </section>
  )
}
