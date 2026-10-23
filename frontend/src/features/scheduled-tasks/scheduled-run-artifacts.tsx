import { useState } from 'react'
import type { ScheduledArtifact } from '@/api/generated.schemas'
import { Button } from '@/components/ui/button'
import { scheduledTaskFailure, useScheduledArtifactDownload, useScheduledRunArtifacts } from './api'

function ArtifactDownload({
  tid,
  issueId,
  runId,
  artifact,
}: {
  tid: string
  issueId: string
  runId: string
  artifact: ScheduledArtifact
}) {
  const download = useScheduledArtifactDownload(tid, issueId, runId)
  return (
    <li className="space-y-1 rounded border p-2">
      <p className="break-all text-xs">
        {artifact.name} · {artifact.kind} · {artifact.sizeBytes} B
      </p>
      <p className="break-all text-xs text-muted-foreground">SHA-256 {artifact.sha256}</p>
      <Button
        type="button"
        size="sm"
        variant="outline"
        disabled={download.isPending}
        onClick={() => {
          download.reset()
          download.mutate(artifact.index)
        }}
      >
        {download.isPending ? '正在获取链接' : '获取下载链接'}
      </Button>
      {download.data && (
        <div className="text-xs">
          <a
            href={download.data.url}
            target="_blank"
            rel="noopener noreferrer"
            referrerPolicy="no-referrer"
            className="underline"
          >
            下载 {artifact.name}
          </a>
          <p className="text-muted-foreground">
            链接有效至 {new Date(download.data.expiresAt).toLocaleString()}，失效后请重新获取。
          </p>
        </div>
      )}
      {download.isError && (
        <p role="alert" className="text-xs text-destructive">
          {scheduledTaskFailure(download.error)}
        </p>
      )}
    </li>
  )
}

function ArtifactList({ tid, issueId, runId }: { tid: string; issueId: string; runId: string }) {
  const artifacts = useScheduledRunArtifacts(tid, issueId, runId)
  if (artifacts.isPending)
    return (
      <p role="status" className="text-xs">
        正在加载产物
      </p>
    )
  if (artifacts.isError)
    return (
      <div role="alert" className="text-xs text-destructive">
        产物列表加载失败
        <Button
          type="button"
          size="sm"
          variant="ghost"
          onClick={() => {
            void artifacts.refetch()
          }}
        >
          重试
        </Button>
      </div>
    )
  if (artifacts.data.artifacts.length === 0)
    return <p className="text-xs text-muted-foreground">暂无可下载产物，文件可能已过期或被清理。</p>
  return (
    <ul aria-label="运行产物" className="space-y-2">
      {artifacts.data.artifacts.map((artifact) => (
        <ArtifactDownload
          key={artifact.index}
          tid={tid}
          issueId={issueId}
          runId={runId}
          artifact={artifact}
        />
      ))}
    </ul>
  )
}

/** Expands one run's current index, keeping short-lived download grants separate from metadata. */
export function ScheduledRunArtifacts({
  tid,
  issueId,
  runId,
}: {
  tid: string
  issueId: string
  runId: string
}) {
  const [expanded, setExpanded] = useState(false)
  return (
    <div className="mt-2 space-y-2">
      <Button
        type="button"
        size="sm"
        variant="ghost"
        aria-expanded={expanded}
        onClick={() => setExpanded(!expanded)}
      >
        {expanded ? '收起产物' : '查看产物'}
      </Button>
      {expanded && <ArtifactList tid={tid} issueId={issueId} runId={runId} />}
    </div>
  )
}
