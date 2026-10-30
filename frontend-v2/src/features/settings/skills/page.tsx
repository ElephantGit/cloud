import { useState } from 'react'
import { Link } from 'react-router'
import { ArrowUpRight, FileText, Plus, Sparkles } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
import { useSettingsData } from '../data/hooks'
import type { Skill } from '../data/types'
import { deleteSkill, saveSkill } from '../data/resource-actions'
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
import { SkillForm } from './skill-form'

type Panel = { kind: 'create' } | { kind: 'detail' | 'edit'; id: string } | null
export default function SkillsPage() {
  const { query, mutation, save } = useSettingsData()
  const [search, setSearch] = useState('')
  const [source, setSource] = useState('all')
  const [status, setStatus] = useState('all')
  const [order, setOrder] = useState('asc')
  const [panel, setPanel] = useState<Panel>(null)
  const [removing, setRemoving] = useState<Skill>()
  const skills = query.data?.skills ?? []
  const selected =
    panel && 'id' in panel ? skills.find((s) => s.id === panel.id) : undefined
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
  const clear = () => {
    setSearch('')
    setSource('all')
    setStatus('all')
  }
  const open = (next: Panel) => {
    mutation.reset()
    setPanel(next)
  }
  const create = () => open({ kind: 'create' })
  return (
    <section>
      <PageHeader
        title="Skills"
        description="整理可复用的说明文档，区分自建技能与插件提供的技能。"
        count={skills.length}
        actions={
          <Button disabled={!query.data || query.isError} onClick={create}>
            <Plus data-icon="inline-start" />
            新建技能
          </Button>
        }
      />
      <DataState
        pending={query.isPending}
        error={query.error}
        retry={() => void query.refetch()}
      >
        <div className="settings-toolbar">
          <SearchInput label="搜索技能" value={search} onChange={setSearch} />
          <Filter
            label="技能来源"
            value={source}
            onChange={setSource}
            options={[
              ['all', '全部来源'],
              ['local', '自建技能'],
              ['plugin', '插件提供'],
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
        <ActionError error={mutation.error} />
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
                        ? '自建技能'
                        : '插件提供 · 只读'}
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
                  {skill.availability === 'available'
                    ? '可用'
                    : '来源插件未安装'}
                </Badge>
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
            <Button onClick={create}>新建技能</Button>
          </NoResults>
        )}
        <div className="settings-list-footer">
          <span>
            显示 {filtered.length} / {skills.length} 个技能 · 共{' '}
            {skills.filter((s) => s.source.kind === 'local').length} 个自建
          </span>
          <span>说明文档，不会执行工具或脚本</span>
        </div>
      </DataState>
      {panel?.kind === 'create' && (
        <DetailPanel
          title="新建技能"
          description="写下清晰的说明，让相同工作有一致的方法。"
          onClose={() => setPanel(null)}
        >
          <SkillForm
            pending={mutation.isPending}
            error={mutation.error}
            onCancel={() => setPanel(null)}
            onSave={(draft) =>
              save(
                (data) => saveSkill(data, draft),
                '演示技能已保存',
                () => setPanel(null),
              )
            }
          />
        </DetailPanel>
      )}
      {selected && panel?.kind === 'edit' && (
        <DetailPanel
          title={`编辑 ${selected.name}`}
          description="修改只保存在此浏览器的演示空间。"
          onClose={() => setPanel(null)}
        >
          <SkillForm
            key={selected.id}
            skill={selected}
            pending={mutation.isPending}
            error={mutation.error}
            onCancel={() => open({ kind: 'detail', id: selected.id })}
            onSave={(draft) =>
              save(
                (data) => saveSkill(data, draft, selected.id),
                '演示技能已保存',
                () => setPanel({ kind: 'detail', id: selected.id }),
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
                ? '自建技能'
                : '插件提供 · 只读'}
            </Badge>
            {selected.source.kind === 'local' && (
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
                    mutation.reset()
                    setRemoving(selected)
                  }}
                >
                  删除技能
                </Button>
              </div>
            )}
          </div>
          <dl className="settings-facts">
            <dt>命名空间</dt>
            <dd>{selected.namespace}</dd>
            <dt>可用状态</dt>
            <dd>
              {selected.availability === 'available'
                ? '可用（演示）'
                : '不可用，来源插件未安装'}
            </dd>
            <dt>来源</dt>
            <dd>
              {selected.source.kind === 'local' ? (
                '当前用户的本机演示空间'
              ) : (
                <Link
                  className="underline underline-offset-4"
                  to={`/settings/plugins?plugin=${encodeURIComponent(selected.source.pluginId)}`}
                >
                  {selected.source.pluginId} ↗
                </Link>
              )}
            </dd>
          </dl>
          {selected.availability === 'unavailable' && (
            <p className="rounded-lg bg-muted p-3 text-sm text-muted-foreground">
              说明仍可预览。重新安装来源插件后，此技能会恢复可用。
            </p>
          )}
          <Separator />
          <MarkdownPreview content={selected.content} />
        </DetailPanel>
      )}
      {removing && (
        <ConfirmDialog
          title={`删除 ${removing.name}？`}
          description="将删除此浏览器中的自建技能和说明文档。此操作不影响插件技能或真实会话。"
          confirmLabel="删除技能"
          pending={mutation.isPending}
          error={mutation.error}
          onClose={() => setRemoving(undefined)}
          onConfirm={() =>
            save(
              (data) => deleteSkill(data, removing.id),
              '演示技能已删除',
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
