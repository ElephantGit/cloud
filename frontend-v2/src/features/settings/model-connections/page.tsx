import { useState } from 'react'
import { Cable, Plus } from 'lucide-react'
import type { ModelConnection } from '@/api/generated.schemas'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Switch } from '@/components/ui/switch'
import { useSettingsData } from '../data/hooks'
import {
  changeConnection,
  connectionReady,
  saveConnection,
  setModelDefault,
} from '../data/resource-actions'
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
import { ConnectionForm } from './connection-form'

export default function ModelConnectionsPage() {
  const { query, mutation, save } = useSettingsData()
  const [search, setSearch] = useState('')
  const [status, setStatus] = useState('all')
  const [editor, setEditor] = useState<{ connection?: ModelConnection }>()
  const [removing, setRemoving] = useState<ModelConnection>()
  const [defaultDraft, setDefaultDraft] = useState('')
  const data = query.data
  const connections = data?.connections ?? []
  const filtered = connections.filter(
    (c) =>
      `${c.name} ${c.baseUrl} ${c.models.map((m) => m.name).join(' ')}`
        .toLowerCase()
        .includes(search.trim().toLowerCase()) &&
      (status === 'all' || c.enabled === (status === 'enabled')),
  )
  const ready = data ? connectionReady(data) : false
  const choices = connections
    .filter((c) => c.enabled && c.credentialConfigured)
    .flatMap((c) =>
      c.models.map((m) => ({
        key: JSON.stringify([c.id, m.id]),
        connectionId: c.id,
        modelId: m.id,
        label: `${c.name} / ${m.name}`,
      })),
    )
  const currentKey = data
    ? JSON.stringify([
        data.modelDefault.connectionId,
        data.modelDefault.modelId,
      ])
    : ''
  const selectedKey = defaultDraft || (ready ? currentKey : '')
  const chosen = choices.find((c) => c.key === selectedKey)
  const open = (connection?: ModelConnection) => {
    mutation.reset()
    setEditor({ connection })
  }
  return (
    <section>
      <PageHeader
        title="模型连接"
        description="管理个人连接与模型目录。这里的设置仅用于演示，不会影响对话。"
        count={connections.length}
        actions={
          <Button disabled={!data || query.isError} onClick={() => open()}>
            <Plus data-icon="inline-start" />
            新建连接
          </Button>
        }
      />
      <DataState
        pending={query.isPending}
        error={query.error}
        retry={() => void query.refetch()}
      >
        {data && (
          <div className="flex flex-col gap-4 border-y py-5">
            <Field>
              <FieldLabel htmlFor="settings-default-model">默认模型</FieldLabel>
              <div className="flex flex-wrap gap-2">
                <NativeSelect
                  id="settings-default-model"
                  value={selectedKey}
                  className="max-w-full flex-1 sm:max-w-md"
                  disabled={mutation.isPending || choices.length === 0}
                  onChange={(e) => setDefaultDraft(e.target.value)}
                >
                  <NativeSelectOption value="">
                    请选择可用的演示模型
                  </NativeSelectOption>
                  {choices.map((c) => (
                    <NativeSelectOption key={c.key} value={c.key}>
                      {c.label}
                    </NativeSelectOption>
                  ))}
                </NativeSelect>
                <Button
                  variant="outline"
                  disabled={
                    !chosen || selectedKey === currentKey || mutation.isPending
                  }
                  onClick={() =>
                    chosen &&
                    save(
                      (next) =>
                        setModelDefault(
                          next,
                          chosen.connectionId,
                          chosen.modelId,
                        ),
                      '演示默认模型已保存',
                      () => setDefaultDraft(''),
                    )
                  }
                >
                  保存默认模型
                </Button>
              </div>
              <FieldDescription>
                {ready
                  ? '仅列出已启用且配置了演示凭据的连接。'
                  : '当前默认选择不可用。请配置演示凭据、启用连接后重新选择。'}
              </FieldDescription>
            </Field>
          </div>
        )}
        <div className="settings-toolbar">
          <SearchInput
            label="搜索模型连接"
            value={search}
            onChange={setSearch}
          />
          <Filter
            label="连接状态"
            value={status}
            onChange={setStatus}
            options={[
              ['all', '全部状态'],
              ['enabled', '已启用'],
              ['disabled', '已停用'],
            ]}
          />
        </div>
        <ActionError error={mutation.error} />
        <div className="flex flex-col gap-4">
          {filtered.map((connection) => (
            <article
              key={connection.id}
              aria-label={connection.name}
              className="rounded-xl border p-5"
            >
              <div className="flex flex-wrap items-start justify-between gap-4">
                <div className="flex min-w-0 items-start gap-3">
                  <Cable
                    className="mt-1 size-5 shrink-0 text-muted-foreground"
                    aria-hidden="true"
                  />
                  <div className="min-w-0">
                    <h2 className="break-words text-sm font-semibold">
                      {connection.name}
                    </h2>
                    <p className="mt-1 break-all text-xs text-muted-foreground">
                      {connection.baseUrl}
                    </p>
                  </div>
                </div>
                <div className="flex items-center gap-2">
                  <Badge variant="outline">
                    {connection.protocol === 'openai-completions'
                      ? 'OpenAI 兼容'
                      : 'Anthropic'}
                  </Badge>
                  <Switch
                    aria-label={`启用 ${connection.name}`}
                    checked={connection.enabled}
                    disabled={mutation.isPending}
                    onCheckedChange={() =>
                      save(
                        (next) =>
                          changeConnection(next, connection.id, 'toggle'),
                        '演示连接状态已更新',
                      )
                    }
                  />
                </div>
              </div>
              <div className="mt-5 flex flex-col gap-2">
                {connection.models.map((model) => (
                  <div
                    key={model.id}
                    className="flex flex-wrap justify-between gap-2 rounded-md bg-muted/50 px-3 py-2 text-xs"
                  >
                    <span className="min-w-0 break-all">
                      {model.name}
                      <span className="ml-2 text-muted-foreground">
                        {model.id}
                      </span>
                    </span>
                    <span className="text-muted-foreground">
                      上下文 {model.contextWindow.toLocaleString()} · 输出{' '}
                      {model.maxTokens.toLocaleString()}
                    </span>
                  </div>
                ))}
              </div>
              <div className="mt-5 flex flex-wrap items-center justify-between gap-3">
                <div className="flex flex-wrap items-center gap-2">
                  <Badge variant="secondary">
                    {connection.credentialConfigured
                      ? '演示凭据已配置'
                      : '未配置演示凭据'}
                  </Badge>
                  <Button
                    variant="ghost"
                    size="sm"
                    disabled={mutation.isPending}
                    onClick={() =>
                      save(
                        (next) =>
                          changeConnection(next, connection.id, 'credential'),
                        connection.credentialConfigured
                          ? '演示凭据状态已清除'
                          : '已模拟配置凭据',
                      )
                    }
                  >
                    {connection.credentialConfigured
                      ? '清除演示凭据'
                      : '模拟配置凭据'}
                  </Button>
                </div>
                <div className="flex gap-2">
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => open(connection)}
                  >
                    编辑连接
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => {
                      mutation.reset()
                      setRemoving(connection)
                    }}
                  >
                    删除
                  </Button>
                </div>
              </div>
            </article>
          ))}
        </div>
        {!filtered.length && (
          <NoResults
            filtered={connections.length > 0}
            onClear={() => {
              setSearch('')
              setStatus('all')
            }}
          >
            <Button onClick={() => open()}>新建连接</Button>
          </NoResults>
        )}
        <p className="mt-5 text-xs leading-relaxed text-muted-foreground">
          仅模拟凭据是否已配置，不接收 API Key，不发送模型请求。
        </p>
      </DataState>
      {editor && (
        <DetailPanel
          title={editor.connection ? '编辑模型连接' : '新建模型连接'}
          description="保存连接元数据与模型目录，不连接外部服务。"
          onClose={() => setEditor(undefined)}
        >
          <ConnectionForm
            key={editor.connection?.id ?? 'new'}
            connection={editor.connection}
            pending={mutation.isPending}
            error={mutation.error}
            onCancel={() => setEditor(undefined)}
            onSave={(draft, expectedVersion) =>
              save(
                (next) =>
                  saveConnection(
                    next,
                    draft,
                    editor.connection?.id,
                    expectedVersion,
                  ),
                '演示连接已保存',
                () => setEditor(undefined),
              )
            }
          />
        </DetailPanel>
      )}
      {removing && (
        <ConfirmDialog
          title={`删除 ${removing.name}？`}
          description="删除此浏览器中的演示连接。若它是默认连接，删除后需要重新选择默认模型。"
          confirmLabel="删除连接"
          pending={mutation.isPending}
          error={mutation.error}
          onClose={() => setRemoving(undefined)}
          onConfirm={() =>
            save(
              (next) => changeConnection(next, removing.id, 'delete'),
              '演示连接已删除',
              () => setRemoving(undefined),
            )
          }
        />
      )}
    </section>
  )
}
