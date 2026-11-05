import { ProjectResults } from './project-results'
import { listMarketSources, syncMarketSource } from '../market-api/generated'
import { MarketSources } from './market-sources'
import { useRef, useState } from 'react'
import { useSearchParams } from 'react-router'
import { toast } from 'sonner'
import { Download, ExternalLink } from 'lucide-react'
import type { PluginCatalogEntry, SpacePlugin } from '@/api/generated.schemas'
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
import { useUser } from '@/app/auth/user'
import { demoPlugin, localWorkflow, withDemoCatalog } from '@/lib/settings-demo/plugin-catalog'
import {
  demoSelectionFor,
  useDemoPluginSelections,
} from '@/lib/settings-demo/plugin-selections'
import { pluginKind, pluginKindOrder } from './plugin-kind'
import { PluginMark } from './plugin-mark'
import { isPreinstalledAgent } from '@/lib/plugin-runtime'
import { installDelayMs, waitForInstallDelay } from './install-delay'

const stateLabels: Record<string, string> = {
  installed: '已安装',
  failed: '安装失败',
  pending: '等待节点',
  installing: '安装中',
  removing: '移除中',
  removed: '未安装',
}

function StateBadge({
  selection,
  demoSelected,
  installing,
}: {
  selection?: SpacePlugin
  demoSelected: boolean
  installing: boolean
}) {
  if (installing) return <Badge variant="outline">安装中</Badge>
  if (demoSelected) return <Badge variant="secondary">已安装</Badge>
  if (selection && isPreinstalledAgent(selection))
    return <Badge variant="outline">运行时预装</Badge>
  const state = selection?.observedState ?? 'removed'
  const label =
    selection?.kind === 'hook' && state === 'installed'
      ? '已安装（演示）'
      : selection?.kind !== 'skill' &&
          selection?.kind !== 'agent' &&
          selection?.kind !== 'workflow' &&
          selection?.kind !== 'hook' &&
          selection?.desiredState === 'installed'
        ? '仅空间选择'
        : (stateLabels[state] ?? state)
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
      {label}
    </Badge>
  )
}

function missingEntry(selection: SpacePlugin): PluginCatalogEntry {
  return {
    id: pluginKey(selection),
    identifier: selection.identifier,
    title: selection.identifier,
    kind: selection.kind ?? 'skill',
    sourceNamespace: selection.sourceNamespace,
    description: '来源不可用。可以移除已有选择。',
    version: selection.desiredVersion,
    indexedAt: selection.updatedAt,
    sourceUrl: '',
    marketplaceVisible: false,
    homepage: null,
    license: null,
    logo: null,
    packMembers: null,
    readme: null,
    sha256: null,
    targets: null,
    url: null,
  }
}

export default function PluginsPage() {
  const user = useUser()
  const market = usePluginMarketplace()
  const demoScope = `${user.id}:${market.tenantId}:${market.spaceId}`
  const demoSelection = useDemoPluginSelections(demoSelectionFor(demoScope))
  const setDemoSelected = useDemoPluginSelections((state) => state.setSelected)
  const [sourcesOpen, setSourcesOpen] = useState(false)
  const [syncing, setSyncing] = useState(false)
  const synchronize = async () => {
    setSyncing(true)
    try {
      const { items } = await listMarketSources(market.tenantId, market.spaceId)
      const failures: string[] = []
      for (const source of [
        'official',
        ...items.filter((s) => s.enabled).map((s) => s.namespace),
      ]) {
        try {
          await syncMarketSource(market.tenantId, market.spaceId, source)
        } catch {
          failures.push(source)
        }
      }
      await market.refetch()
      if (failures.length) toast.error('部分来源同步失败，已保留上次目录。')
      else toast.success('市场目录已同步')
    } catch {
      toast.error('未能同步市场，请重试。')
    } finally {
      setSyncing(false)
    }
  }
  const [search, setSearch] = useState('')
  const [kind, setKind] = useState('all')
  const [status, setStatus] = useState('all')
  const [order, setOrder] = useState('asc')
  const [pendingInstalls, setPendingInstalls] = useState<Set<string>>(
    () => new Set(),
  )
  const pendingInstallsRef = useRef(new Set<string>())
  const [removing, setRemoving] = useState<{
    entry: PluginCatalogEntry
    selection?: SpacePlugin
    demo: boolean
  }>()
  const [params, setParams] = useSearchParams()
  const availableCatalog = withDemoCatalog(market.catalog)
  const available = new Set(availableCatalog.map(pluginKey))
  const catalog = [
    ...availableCatalog,
    ...market.installations
      .filter(
        (s) =>
          (s.desiredState === 'installed' ||
            s.observedState === 'removing' ||
            s.observedState === 'failed') &&
          !available.has(pluginKey(s)),
      )
      .map(missingEntry),
  ]
  const selected = catalog.find(
    (entry) => pluginKey(entry) === params.get('plugin'),
  )
  const serverSelection = (entry: PluginCatalogEntry) =>
    localWorkflow(entry) ? undefined : market.selection(entry)
  const selectedSelection = selected ? serverSelection(selected) : undefined
  const demoInstallable = (entry: PluginCatalogEntry) => {
    const selection = serverSelection(entry)
    return (
      demoPlugin(entry) ||
      (selection != null && isPreinstalledAgent(selection))
    )
  }
  const demoSelected = (entry: PluginCatalogEntry) =>
    demoInstallable(entry) && demoSelection.includes(pluginKey(entry))
  const filtered = catalog
    .filter((entry) => {
      const selection = serverSelection(entry)
      return (
        `${entry.title} ${entry.description} ${pluginKey(entry)}`
          .toLowerCase()
          .includes(search.trim().toLowerCase()) &&
        (kind === 'all' || entry.kind === kind) &&
        (status === 'all' ||
          (status === 'installed'
            ? demoInstallable(entry)
              ? demoSelected(entry)
              : pluginInstalled(selection)
            : status === 'failed'
              ? selection?.observedState === 'failed'
              : demoInstallable(entry)
                ? !demoSelected(entry)
                : !selection || selection.observedState === 'removed'))
      )
    })
    .sort(
      (a, b) =>
        (order === 'asc' ? 1 : -1) * a.title.localeCompare(b.title, 'zh-CN'),
    )
  const knownKinds = new Set(pluginKindOrder)
  const orderedKinds = [
    ...pluginKindOrder,
    ...new Set(
      filtered
        .map((entry) => entry.kind)
        .filter((kindValue) => !knownKinds.has(kindValue)),
    ),
  ]
  const groups = orderedKinds
    .map((kindValue) => ({
      kind: kindValue,
      meta: pluginKind({ kind: kindValue as PluginCatalogEntry['kind'] }),
      items: filtered.filter((entry) => entry.kind === kindValue),
    }))
    .filter((group) => group.items.length > 0)
  const installedSelections = market.installations.filter(
    (selection) => !localWorkflow(selection) && pluginInstalled(selection),
  )
  const installedKeys = new Set([
    ...installedSelections.map(pluginKey),
    ...catalog.filter(demoSelected).map(pluginKey),
  ])
  const clear = () => {
    setSearch('')
    setKind('all')
    setStatus('all')
  }
  const open = (entry: PluginCatalogEntry) => {
    setParams({ plugin: pluginKey(entry) })
  }
  const installing = (entry: PluginCatalogEntry) =>
    pendingInstalls.has(pluginKey(entry))
  const install = async (entry: PluginCatalogEntry) => {
    const key = pluginKey(entry)
    if (pendingInstallsRef.current.has(key)) return
    pendingInstallsRef.current.add(key)
    setPendingInstalls((current) => new Set(current).add(key))
    market.resetErrors()
    const delay = waitForInstallDelay(installDelayMs())
    try {
      if (demoInstallable(entry)) {
        await delay
        setDemoSelected(demoScope, key, true)
        toast.success(`已安装 ${entry.title}`)
      } else {
        await Promise.all([market.install(entry), delay])
        const message =
          entry.kind === 'hook'
            ? `已记录演示安装 ${entry.title}`
            : `${entry.kind === 'workflow' ? '已安装' : '已选择'} ${entry.title}`
        toast.success(message)
      }
    } catch {
      await delay
    } finally {
      pendingInstallsRef.current.delete(key)
      setPendingInstalls((current) => {
        const next = new Set(current)
        next.delete(key)
        return next
      })
    }
  }
  const requestRemove = (entry: PluginCatalogEntry) => {
    market.resetErrors()
    if (demoPlugin(entry)) {
      if (demoSelected(entry))
        setRemoving({ entry, demo: true })
      return
    }
    const selection = serverSelection(entry)
    if (selection) setRemoving({ entry, selection, demo: false })
  }
  const confirmRemove = () => {
    if (!removing) return
    if (removing.demo) {
      setDemoSelected(demoScope, pluginKey(removing.entry), false)
      toast.success(`已卸载 ${removing.entry.title}`)
      setRemoving(undefined)
      return
    }
    if (!removing.selection) return
    void market
      .remove(removing.selection)
      .then(() => {
        toast.success(
          removing.entry.kind === 'hook'
            ? `已移除演示 Hook ${removing.entry.title}`
            : `已提交移除 ${removing.entry.title}`,
        )
        setRemoving(undefined)
      })
      .catch(() => undefined)
  }
  function actions(entry: PluginCatalogEntry) {
    const selection = serverSelection(entry)
    const pending = installing(entry)
    const busy =
      market.mutating ||
      pending ||
      selection?.observedState === 'installing' ||
      selection?.observedState === 'removing'
    if (pending)
      return (
        <Button size="sm" variant="secondary" disabled>
          <Download data-icon="inline-start" />
          安装中…
        </Button>
      )
    if (demoPlugin(entry))
      return demoSelected(entry) ? (
        <Button
          size="sm"
          variant="outline"
          disabled={!market.canManage}
          onClick={() => requestRemove(entry)}
        >
          {entry.kind === 'workflow' ? '卸载' : '移除'}
        </Button>
      ) : (
        <Button
          size="sm"
          variant="secondary"
          disabled={!market.canManage || pending}
          onClick={() => void install(entry)}
        >
          <Download data-icon="inline-start" />
          {pending ? '安装中…' : '安装'}
        </Button>
      )
    if (selection && isPreinstalledAgent(selection))
      return demoSelected(entry) ? null : (
        <Button
          size="sm"
          variant="secondary"
          disabled={!market.canManage || pending}
          onClick={() => void install(entry)}
        >
          <Download data-icon="inline-start" />
          {pending ? '安装中…' : '安装'}
        </Button>
      )
    if (
      selection?.desiredState === 'removed' &&
      selection.observedState === 'failed'
    )
      return (
        <Button
          size="sm"
          variant="outline"
          disabled={!market.canManage || market.mutating}
          onClick={() => requestRemove(entry)}
        >
          重试移除
        </Button>
      )
    if (entry.kind === 'hook' && selection?.observedState === 'failed')
      return (
        <Button
          size="sm"
          variant="secondary"
          disabled={!market.canManage || busy}
          onClick={() => void install(entry)}
        >
          <Download data-icon="inline-start" />
          {pending ? '安装中…' : '重试安装'}
        </Button>
      )
    if (
      (!available.has(pluginKey(entry)) || (entry.kind !== 'skill' && entry.kind !== 'workflow')) &&
      selection?.desiredState === 'installed'
    )
      return (
        <Button
          size="sm"
          variant="outline"
          disabled={!market.canManage || market.mutating}
          onClick={() => requestRemove(entry)}
        >
          {entry.kind === 'workflow' ? '卸载' : '移除'}
        </Button>
      )
    if (entry.kind === 'pack')
      return (
        <span className="text-xs text-muted-foreground">合集安装尚未支持</span>
      )
    if (
      entry.kind !== 'skill' &&
      entry.kind !== 'agent' &&
      entry.kind !== 'workflow' &&
      entry.kind !== 'hook' &&
      selection?.desiredState !== 'installed'
    )
      return (
        <span className="text-xs text-muted-foreground">
          当前节点不支持此类型
        </span>
      )
    return selection?.desiredState === 'installed' ? (
      <>
        {(entry.kind === 'skill' || entry.kind === 'workflow') &&
          (entry.version !== selection.desiredVersion ||
            selection.observedState === 'failed' ||
            selection.observedState === 'pending') && (
            <Button
              disabled={!market.canManage || busy}
              onClick={() => void install(entry)}
              size="sm"
              variant="secondary"
            >
              {pending
                ? '安装中…'
                : entry.version !== selection.desiredVersion
                  ? '更新'
                  : '重新应用'}
            </Button>
          )}
        <Button
          size="sm"
          variant="outline"
          disabled={!market.canManage || busy}
          onClick={() => requestRemove(entry)}
        >
          {entry.kind === 'workflow' ? '卸载' : '移除'}
        </Button>
      </>
    ) : (
      <Button
        size="sm"
        variant={
          selection?.observedState === 'failed' ? 'outline' : 'secondary'
        }
        disabled={!market.canManage || busy}
        onClick={() => void install(entry)}
      >
        <Download data-icon="inline-start" />
        {pending
          ? '安装中…'
          : selection?.observedState === 'failed'
            ? '重试安装'
            : '安装'}
      </Button>
    )
  }
  return (
    <section>
      <PageHeader
        title="Plugins"
        description="浏览空间插件，查看详情并管理安装状态。"
        count={catalog.length}
        actions={
          <>
            <Button variant="outline" onClick={() => setSourcesOpen(true)}>
              管理市场
            </Button>
            {market.canManage && (
              <Button
                variant="secondary"
                disabled={syncing || !market.spaceId}
                onClick={() => void synchronize()}
              >
                {syncing ? '同步中…' : '同步'}
              </Button>
            )}
          </>
        }
      />
      <DataState
        pending={market.pending}
        error={market.error}
        retry={() => void market.refetch()}
      >
        {!market.canManage && (
          <p className="text-sm text-muted-foreground">
            当前账号为只读成员；安装与移除需要工作区 owner 权限。
          </p>
        )}
        <p className="text-sm text-muted-foreground">
          MCP 与运行时预装 Agent 的安装只改变当前浏览器中的状态；Hook
          的安装/卸载只写入服务端空间选择，不下载或执行包；市场 Agent
          的安装只记录空间选择。
        </p>
        <div className="settings-toolbar">
          {market.spaces.length > 1 && (
            <Filter
              label="空间"
              value={market.spaceId}
              onChange={(id) => {
                market.chooseSpace(id)
                setSourcesOpen(false)
                setRemoving(undefined)
                setParams({})
              }}
              options={market.spaces.map((s) => [s.id, s.name] as const)}
            />
          )}
          <SearchInput label="搜索插件" value={search} onChange={setSearch} />
          <Filter
            label="插件类型"
            value={kind}
            onChange={setKind}
            options={[
              ['all', '全部类型'],
              ...[...new Set(catalog.map((entry) => entry.kind))].map(
                (value) => [value, pluginKind({ kind: value }).label] as const,
              ),
            ]}
          />
          <Filter
            label="安装状态"
            value={status}
            onChange={setStatus}
            options={[
              ['all', '全部状态'],
              ['installed', '已安装 / 已选择'],
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
        <ActionError error={market.mutationError} />
        <div className="settings-plugin-groups">
          {groups.map((group) => {
            const GroupIcon = group.meta.icon
            return (
              <section
                key={group.kind}
                className="settings-plugin-group"
                aria-labelledby={`plugin-group-${group.kind}`}
              >
                <header className="settings-plugin-group-header">
                  <span className="settings-plugin-group-icon">
                    <GroupIcon aria-hidden="true" />
                  </span>
                  <span>
                    <span className="flex items-center gap-2">
                      <h2 id={`plugin-group-${group.kind}`}>
                        {group.meta.label}
                      </h2>
                      <Badge variant="secondary">{group.items.length}</Badge>
                    </span>
                    <span>{group.meta.groupDescription}</span>
                  </span>
                </header>
                <div className="settings-plugin-grid">
                  {group.items.map((entry) => {
                    const meta = pluginKind(entry)
                    const KindIcon = meta.icon
                    return (
                      <article
                        key={pluginKey(entry)}
                        aria-label={entry.title}
                        className="settings-plugin-card"
                      >
                        <div className="settings-plugin-card-header">
                          <button
                            type="button"
                            className="settings-plugin-identity"
                            onClick={() => open(entry)}
                            aria-label={`查看插件 ${entry.title}`}
                          >
                            <PluginMark entry={entry} />
                            <span className="min-w-0">
                              <span className="settings-plugin-title">
                                {entry.title}
                              </span>
                              <span className="settings-plugin-id">
                                {pluginKey(entry)}
                              </span>
                            </span>
                          </button>
                          <Badge
                            variant="secondary"
                            className="settings-plugin-kind"
                          >
                            <KindIcon data-icon="inline-start" />
                            {meta.label}
                          </Badge>
                        </div>
                        <p className="settings-plugin-description">
                          {entry.description}
                        </p>
                        <div className="settings-plugin-card-footer">
                          <span className="settings-plugin-version">
                            {entry.sourceNamespace} · v{entry.version}
                          </span>
                          <div className="settings-resource-actions">
                            <StateBadge
                              selection={serverSelection(entry)}
                              demoSelected={demoSelected(entry)}
                              installing={installing(entry)}
                            />
                            {actions(entry)}
                          </div>
                        </div>
                      </article>
                    )
                  })}
                </div>
              </section>
            )
          })}
        </div>
        {!filtered.length && (
          <NoResults filtered={catalog.length > 0} onClear={clear} />
        )}
        <div className="settings-list-footer">
          <span>
            显示 {filtered.length} / {catalog.length} 个插件 · 共{' '}
            {installedKeys.size} 个已安装或已选择
          </span>
        </div>
      </DataState>
      {params.has('plugin') && !market.pending && !selected && (
        <p role="alert" className="mt-4 text-sm text-muted-foreground">
          找不到该插件。
          <Button variant="link" onClick={() => setParams({})}>
            返回全部插件
          </Button>
        </p>
      )}
      {sourcesOpen && (
        <MarketSources
          tenant={market.tenantId}
          space={market.spaceId}
          owner={market.canManage}
          officialUrl={
            market.catalog.find(
              (entry) => entry.sourceNamespace === 'official',
            )?.sourceUrl
          }
          officialSyncedAt={market.syncedAt}
          onClose={() => setSourcesOpen(false)}
          refresh={market.refetch}
        />
      )}
      {selected && (
        <DetailPanel
          title={selected.title}
          description={selected.description}
          onClose={() => setParams({}, { replace: true })}
        >
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div className="settings-plugin-detail-identity">
              <PluginMark entry={selected} large />
              <span className="settings-plugin-detail-meta">
                <Badge variant="outline">{pluginKind(selected).label}</Badge>
                <span>{pluginKey(selected)}</span>
              </span>
              <StateBadge
                selection={selectedSelection}
                demoSelected={demoSelected(selected)}
                installing={installing(selected)}
              />
            </div>
            {actions(selected)}
          </div>
          <ActionError error={market.mutationError} />
          {selected.kind === 'skill' && market.selection(selected) && (
            <ProjectResults
              tenant={market.tenantId}
              space={market.spaceId}
              namespace={selected.sourceNamespace}
              identifier={selected.identifier}
            />
          )}
          <dl className="settings-facts">
            <dt>插件标识</dt>
            <dd>{pluginKey(selected)}</dd>
            <dt>类型</dt>
            <dd>{pluginKind(selected).label}</dd>
            <dt>目录版本</dt>
            <dd>
              {available.has(pluginKey(selected))
                ? selected.version
                : '来源不可用'}
            </dd>
            {localWorkflow(selected) ? (
              <>
                <dt>安装状态</dt>
                <dd>{demoSelected(selected) ? `已安装 ${selected.version}` : '未安装'}</dd>
              </>
            ) : (
              <>
                <dt>安装选择</dt>
                <dd>
                  {demoSelected(selected)
                    ? `本地安装 ${selected.version}`
                    : selectedSelection && isPreinstalledAgent(selectedSelection)
                      ? '运行时预装，未开始本地安装'
                      : selectedSelection?.desiredState === 'installed'
                      ? `安装 ${selectedSelection?.desiredVersion}`
                      : '未选择'}
                </dd>
                <dt>安装结果</dt>
                <dd>
                  {demoSelected(selected)
                    ? '浏览器本地状态'
                    : selected.kind === 'hook' &&
                        selectedSelection?.observedState === 'installed'
                      ? '服务端已记录（演示，不执行）'
                      : selectedSelection && isPreinstalledAgent(selectedSelection)
                        ? '运行时预装（服务端未变更）'
                        : (selectedSelection?.observedVersion ?? '尚未安装')}
                </dd>
              </>
            )}
            <dt>来源</dt>
            <dd>
              {selected.sourceNamespace} · {selected.sourceUrl}
            </dd>
            <dt>许可证</dt>
            <dd>{selected.license ?? '未提供'}</dd>
          </dl>
          {selectedSelection?.installError && (
            <p role="alert" className="text-sm text-destructive">
              {selectedSelection?.installError}
            </p>
          )}
          {!localWorkflow(selected) && (
            <p className="rounded-lg bg-muted p-3 text-xs leading-relaxed text-muted-foreground">
              {demoPlugin(selected)
                ? '此插件类型在当前 v2-server 中没有执行器；安装和卸载只改变浏览器本地状态，不会下载或运行插件包。'
                : selected.kind === 'hook'
                  ? '此 Hook 仅用于演示安装和卸载；状态保存在服务端空间选择中，不会下载、注册或执行 Hook 包。'
                  : selected.kind === 'workflow'
                    ? '安装会导入工作流并发布初始版本，可在工作流库中运行。'
                    : selectedSelection && isPreinstalledAgent(selectedSelection)
                      ? '该 Agent 由运行时预装；安装只改变浏览器本地状态，不写服务端，也不影响执行器候选。'
                      : '空间选择适用于该空间的项目。 只有项目节点确认后才显示已安装。'}
            </p>
          )}
          <Separator />
          {selected.readme ? (
            <MarkdownPreview content={selected.readme} />
          ) : (
            <p className="text-sm text-muted-foreground">
              该插件未提供 README。
            </p>
          )}
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
          title={`${removing.entry.kind === 'workflow' ? '卸载' : '移除'} ${removing.entry.title}？`}
          description={
            removing.entry.kind === 'workflow'
              ? removing.demo ? '卸载后可以重新安装。' : '已导入的工作流和运行历史会保留。'
              : removing.demo
                ? '将清除当前浏览器中的安装状态，不会卸载真实插件包。'
                : removing.entry.kind === 'hook'
                  ? '将从空间移除该演示 Hook 选择；不会下载、执行或卸载任何真实 Hook 包。'
                  : removing.entry.kind === 'skill'
                    ? '将请求项目节点卸载该技能包，完成后不再提供其中的技能。'
                    : '将从空间移除该插件选择。此操作不会卸载其他工作空间或 Agent 运行时中的包。'
          }
          confirmLabel={removing.entry.kind === 'workflow' ? '卸载' : '移除'}
          pending={removing.demo ? false : market.mutating}
          error={removing.demo ? undefined : market.mutationError}
          onClose={() => setRemoving(undefined)}
          onConfirm={confirmRemove}
        />
      )}
    </section>
  )
}
