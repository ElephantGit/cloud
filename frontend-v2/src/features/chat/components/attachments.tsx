import { FileIcon, PuzzleIcon, XIcon } from 'lucide-react'
import {
  Attachment,
  AttachmentAction,
  AttachmentActions,
  AttachmentContent,
  AttachmentDescription,
  AttachmentGroup,
  AttachmentMedia,
  AttachmentTitle,
} from '@/components/ui/attachment'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
import type { Attachment as AttachmentData, PluginRef } from '../model/types'
import { formatBytes } from './format'

/** Attached files as chips; images show a thumbnail. `onRemove` adds a remove button to each. */
export function AttachmentList({ files, onRemove, className }: { files: AttachmentData[]; onRemove?: (id: string) => void; className?: string }) {
  if (!files.length) return null
  return (
    <AttachmentGroup aria-label="附件" className={className}>
      {files.map((f) => (
        <Attachment key={f.id} size="sm">
          {f.url && f.mimeType.startsWith('image/') ? (
            <AttachmentMedia variant="image">
              <img src={f.url} alt={f.name} />
            </AttachmentMedia>
          ) : (
            <AttachmentMedia>
              <FileIcon />
            </AttachmentMedia>
          )}
          <AttachmentContent>
            <AttachmentTitle>{f.name}</AttachmentTitle>
            <AttachmentDescription>{formatBytes(f.size)}</AttachmentDescription>
          </AttachmentContent>
          {onRemove && (
            <AttachmentActions>
              <AttachmentAction aria-label={`移除 ${f.name}`} onClick={() => onRemove(f.id)}>
                <XIcon />
              </AttachmentAction>
            </AttachmentActions>
          )}
        </Attachment>
      ))}
    </AttachmentGroup>
  )
}

/** Plugins applied to a prompt. `onRemove` makes each removable. */
export function PluginList({ items, onRemove, className }: { items: PluginRef[]; onRemove?: (id: string) => void; className?: string }) {
  if (!items.length) return null
  return (
    <div className={cn('flex flex-wrap gap-1.5', className)} aria-label="已选插件">
      {items.map((p) => (
        <Badge key={p.id} variant="secondary" className="gap-1">
          <PuzzleIcon />
          {p.name}
          {onRemove && (
            <button type="button" aria-label={`移除插件 ${p.name}`} className="-mr-1 rounded-sm hover:text-foreground" onClick={() => onRemove(p.id)}>
              <XIcon className="size-3" />
            </button>
          )}
        </Badge>
      ))}
    </div>
  )
}
