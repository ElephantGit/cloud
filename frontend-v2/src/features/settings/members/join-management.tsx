import { useEffect, useId, useState } from 'react'
import { useSearchParams } from 'react-router'
import { Copy, Link2, Plus } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { useSettingsData } from '../data/hooks'
import {
  createLink,
  decideRequest,
  linkState,
  revokeLink,
} from '../data/member-actions'
import type { DemoLink } from '../data/types'
import { ActionError, Filter, NoResults } from '../components/page-parts'
import { DetailPanel } from '../components/detail-panel'
import { ConfirmDialog } from '../components/confirm-dialog'

const dateLabel = (value: string) =>
  new Date(value).toLocaleString('zh-CN', {
    dateStyle: 'medium',
    timeStyle: 'short',
  })
export function JoinManagement({ view }: { view: 'links' | 'requests' }) {
  const { query, mutation, save, user, canManage } = useSettingsData()
  const [creating, setCreating] = useState(false)
  const [kind, setKind] = useState<DemoLink['kind']>('invite')
  const [note, setNote] = useState('')
  const [revoking, setRevoking] = useState<DemoLink>()
  const [requestFilter, setRequestFilter] = useState('pending')
  const [params, setParams] = useSearchParams()
  const [copyStatus, setCopyStatus] = useState('')
  const [now, setNow] = useState(() => Date.now())
  const id = useId()
  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), 30_000)
    return () => clearInterval(timer)
  }, [])
  const data = query.data
  if (!data || !canManage) return null
  const selected = data.links.find((l) => l.id === params.get('preview'))
  const requests = data.requests.filter(
    (r) =>
      requestFilter === 'all' ||
      (requestFilter === 'pending'
        ? r.status === 'pending'
        : r.status !== 'pending'),
  )
  const previewUrl = selected
    ? new URL(
        `/settings/members?preview=${encodeURIComponent(selected.id)}`,
        window.location.origin,
      ).href
    : ''
  const openPreview = (linkId: string) => {
    setCopyStatus('')
    setParams({ preview: linkId })
  }
  async function copy() {
    try {
      await navigator.clipboard.writeText(previewUrl)
      setCopyStatus('已复制本机演示预览链接')
    } catch {
      setCopyStatus('复制失败，请选中链接后手动复制。')
    }
  }
  return (
    <div className="flex flex-col gap-4">
      <ActionError error={mutation.error} />
      {view === 'links' ? (
        <>
          <div className="flex flex-wrap items-center justify-between gap-3">
            <p className="text-sm text-muted-foreground">
              邀请直接加入；申请需管理员审批。这里只演示管理状态。
            </p>
            <Button
              variant="outline"
              onClick={() => {
                mutation.reset()
                setKind('invite')
                setNote('')
                setCreating(true)
              }}
            >
              <Plus data-icon="inline-start" />
              生成演示链接
            </Button>
          </div>
          {data.links.map((link) => (
            <article
              key={link.id}
              className="flex flex-wrap items-center justify-between gap-4 rounded-lg border p-4"
            >
              <div className="flex min-w-0 items-start gap-3">
                <Link2
                  className="mt-1 size-4 shrink-0 text-muted-foreground"
                  aria-hidden="true"
                />
                <div className="min-w-0">
                  <h3 className="break-words text-sm font-medium">
                    {link.note ||
                      (link.kind === 'invite' ? '演示邀请' : '演示申请')}
                  </h3>
                  <p className="mt-1 text-xs text-muted-foreground">
                    {link.kind === 'invite' ? '邀请链接' : '申请链接'} ·{' '}
                    {dateLabel(link.expiresAt)} 到期
                  </p>
                  {link.kind === 'apply' && (
                    <p className="mt-2 text-xs text-muted-foreground">
                      待审批{' '}
                      {
                        data.requests.filter(
                          (r) => r.linkId === link.id && r.status === 'pending',
                        ).length
                      }{' '}
                      · 已批准{' '}
                      {
                        data.requests.filter(
                          (r) =>
                            r.linkId === link.id && r.status === 'approved',
                        ).length
                      }
                    </p>
                  )}
                </div>
              </div>
              <div className="flex flex-wrap items-center gap-2">
                <Badge variant="outline">{linkState(link, now)}</Badge>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => openPreview(link.id)}
                >
                  预览与复制
                </Button>
                {linkState(link, now) === '有效' && (
                  <Button
                    variant="ghost"
                    size="sm"
                    disabled={mutation.isPending}
                    onClick={() => {
                      mutation.reset()
                      setRevoking(link)
                    }}
                  >
                    撤销
                  </Button>
                )}
              </div>
            </article>
          ))}
          {!data.links.length && (
            <NoResults filtered={false} onClear={() => {}}>
              <Button onClick={() => setCreating(true)}>生成演示链接</Button>
            </NoResults>
          )}
        </>
      ) : (
        <>
          <div className="flex items-center justify-between gap-3">
            <h2 className="text-sm font-medium">加入申请</h2>
            <Filter
              label="申请状态"
              value={requestFilter}
              onChange={setRequestFilter}
              options={[
                ['pending', '待审批'],
                ['processed', '已处理'],
                ['all', '全部申请'],
              ]}
            />
          </div>
          {requests.map((request) => (
            <article
              key={request.id}
              className="flex flex-wrap items-center justify-between gap-4 rounded-lg border p-4"
            >
              <div>
                <h3 className="text-sm font-medium">{request.displayName}</h3>
                <p className="mt-2 text-xs text-muted-foreground">
                  {request.linkNote || '演示申请'} ·{' '}
                  {dateLabel(request.createdAt)}
                </p>
                {request.decidedAt && (
                  <p className="mt-2 text-xs text-muted-foreground">
                    由 {request.decidedByName} 于 {dateLabel(request.decidedAt)}{' '}
                    处理
                  </p>
                )}
              </div>
              {request.status === 'pending' ? (
                <div className="flex gap-2">
                  <Button
                    size="sm"
                    disabled={mutation.isPending}
                    onClick={() =>
                      save(
                        (next) => decideRequest(next, user, request.id, true),
                        '演示申请已批准，成员列表已更新',
                      )
                    }
                  >
                    批准
                  </Button>
                  <Button
                    size="sm"
                    variant="outline"
                    disabled={mutation.isPending}
                    onClick={() =>
                      save(
                        (next) => decideRequest(next, user, request.id, false),
                        '演示申请已拒绝',
                      )
                    }
                  >
                    拒绝
                  </Button>
                </div>
              ) : (
                <Badge variant="secondary">
                  {request.status === 'approved'
                    ? '已批准'
                    : request.status === 'rejected'
                      ? '已拒绝'
                      : request.status}
                </Badge>
              )}
            </article>
          ))}
          {!requests.length && (
            <NoResults
              filtered={requestFilter !== 'all' && data.requests.length > 0}
              onClear={() => setRequestFilter('all')}
            />
          )}
        </>
      )}
      {creating && (
        <DetailPanel
          title="生成演示链接"
          description="链接只能预览此浏览器中的演示记录，不能邀请真实用户。"
          onClose={() => setCreating(false)}
        >
          <form
            className="flex flex-col gap-6"
            onSubmit={(event) => {
              event.preventDefault()
              let createdId = ''
              save(
                (next) => {
                  createdId = createLink(next, user, kind, note)
                },
                '演示链接已生成',
                () => {
                  setCreating(false)
                  openPreview(createdId)
                },
              )
            }}
          >
            <FieldGroup>
              <Field>
                <FieldLabel htmlFor={id + '-kind'}>链接类型</FieldLabel>
                <NativeSelect
                  id={id + '-kind'}
                  value={kind}
                  disabled={mutation.isPending}
                  onChange={(e) =>
                    setKind(e.target.value === 'invite' ? 'invite' : 'apply')
                  }
                >
                  <NativeSelectOption value="invite">
                    邀请链接
                  </NativeSelectOption>
                  <NativeSelectOption value="apply">
                    申请链接
                  </NativeSelectOption>
                </NativeSelect>
              </Field>
              <Field>
                <FieldLabel htmlFor={id + '-note'}>备注</FieldLabel>
                <Input
                  id={id + '-note'}
                  maxLength={200}
                  value={note}
                  disabled={mutation.isPending}
                  onChange={(e) => setNote(e.target.value)}
                  placeholder="用于辨认此链接，不绑定接收人"
                />
              </Field>
            </FieldGroup>
            <p className="text-sm text-muted-foreground">
              有效期 7 天。复制结果为本机演示预览地址，没有真实邀请令牌。
            </p>
            <ActionError error={mutation.error} />
            <div className="settings-form-actions">
              <Button
                type="button"
                variant="outline"
                onClick={() => setCreating(false)}
                disabled={mutation.isPending}
              >
                取消
              </Button>
              <Button type="submit" disabled={mutation.isPending}>
                生成演示链接
              </Button>
            </div>
          </form>
        </DetailPanel>
      )}
      {selected && (
        <DetailPanel
          title="本机演示链接预览"
          description="这个地址只预览当前用户在此浏览器中的记录，不会让任何人加入真实工作空间。"
          onClose={() => setParams({}, { replace: true })}
        >
          <Badge variant="outline">{linkState(selected, now)}</Badge>
          <dl className="settings-facts">
            <dt>类型</dt>
            <dd>{selected.kind === 'invite' ? '邀请' : '申请'}</dd>
            <dt>备注</dt>
            <dd>{selected.note || '无'}</dd>
            <dt>创建者</dt>
            <dd>{selected.createdByName}</dd>
            <dt>到期时间</dt>
            <dd>{dateLabel(selected.expiresAt)}</dd>
          </dl>
          <Field>
            <FieldLabel htmlFor={id + '-preview'}>
              仅本机可用的预览链接
            </FieldLabel>
            <Input
              id={id + '-preview'}
              readOnly
              value={previewUrl}
              onFocus={(event) => event.target.select()}
            />
          </Field>
          <Button variant="outline" onClick={() => void copy()}>
            <Copy data-icon="inline-start" />
            复制演示预览链接
          </Button>
          {copyStatus && (
            <p role="status" className="text-sm text-muted-foreground">
              {copyStatus}
            </p>
          )}
        </DetailPanel>
      )}
      {params.has('preview') && !selected && (
        <p role="alert" className="text-sm text-muted-foreground">
          此浏览器没有该演示链接，或数据已被重置。
          <Button variant="link" onClick={() => setParams({})}>
            返回
          </Button>
        </p>
      )}
      {revoking && (
        <ConfirmDialog
          title="撤销演示链接？"
          description="演示记录将标记为已撤销，已有成员和已处理的申请不受影响。"
          confirmLabel="撤销链接"
          pending={mutation.isPending}
          error={mutation.error}
          onClose={() => setRevoking(undefined)}
          onConfirm={() =>
            save(
              (next) => revokeLink(next, user, revoking.id),
              '演示链接已撤销',
              () => setRevoking(undefined),
            )
          }
        />
      )}
    </div>
  )
}
