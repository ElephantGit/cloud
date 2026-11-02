import { useRef, useState } from 'react'
import { Upload } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import type { Role } from '../data/types'
import { ActionError } from '../components/page-parts'
import {
  MAX_ROLE_MARKDOWN_BYTES,
  previewRoleImport,
  type RoleImportDecision,
  type RoleImportPreview,
} from './import'

export function RoleImportDialog({
  roles,
  pending,
  error,
  onClose,
  onCommit,
}: {
  roles: readonly Role[]
  pending: boolean
  error: unknown
  onClose: () => void
  onCommit: (
    preview: RoleImportPreview,
    decision: RoleImportDecision | null,
    done: () => void,
  ) => void
}) {
  const input = useRef<HTMLInputElement>(null)
  const [preview, setPreview] = useState<RoleImportPreview | null>(null)
  const [decision, setDecision] = useState<RoleImportDecision | null>(null)
  const [reading, setReading] = useState(false)
  const [fileError, setFileError] = useState<unknown>(null)
  const conflict = preview !== null && preview.existing !== null
  const close = () => {
    if (!pending) onClose()
  }
  const choose = async (file: File | undefined) => {
    if (!file) return
    setFileError(null)
    setPreview(null)
    setDecision(null)
    if (!file.name.toLowerCase().endsWith('.md')) {
      setFileError(new Error('请选择一个 .md Markdown 文件。'))
      return
    }
    if (file.size > MAX_ROLE_MARKDOWN_BYTES) {
      setFileError(new Error('Markdown 文件不能超过 1 MiB。'))
      return
    }
    setReading(true)
    try {
      setPreview(previewRoleImport(await file.text(), file.name, roles))
    } catch (cause) {
      setFileError(cause)
    } finally {
      setReading(false)
    }
  }
  const canCommit =
    preview !== null && (!conflict || decision !== null) && !pending
  return (
    <Dialog open onOpenChange={(open) => !open && close()}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>导入角色</DialogTitle>
          <DialogDescription>
            选择一个本地 Markdown 文件。front matter 需要包含 name 和
            description，导入前会检查同名角色。
          </DialogDescription>
        </DialogHeader>
        <input
          ref={input}
          className="hidden"
          type="file"
          accept=".md,text/markdown"
          aria-label="角色 Markdown 文件"
          onChange={(event) => {
            void choose(event.target.files?.[0])
            event.target.value = ''
          }}
        />
        {preview === null ? (
          <Button
            variant="outline"
            disabled={reading}
            onClick={() => input.current?.click()}
          >
            <Upload data-icon="inline-start" />
            {reading ? '正在读取…' : '选择 Markdown 文件'}
          </Button>
        ) : (
          <div className="space-y-3 rounded-lg border p-4">
            <div>
              <p className="text-sm font-medium">{preview.draft.name}</p>
              <p className="text-xs text-muted-foreground">
                {preview.fileName}
              </p>
            </div>
            <p className="text-sm text-muted-foreground">
              {preview.draft.description}
            </p>
            {preview.existing ? (
              <div className="space-y-2">
                <p className="text-xs text-muted-foreground">
                  现有描述：{preview.existing.description || '暂无描述'}
                </p>
                <div className="flex gap-2">
                  <Button
                    size="sm"
                    variant={decision === 'skip' ? 'secondary' : 'outline'}
                    onClick={() => setDecision('skip')}
                  >
                    跳过
                  </Button>
                  <Button
                    size="sm"
                    variant={decision === 'overwrite' ? 'secondary' : 'outline'}
                    onClick={() => setDecision('overwrite')}
                  >
                    覆盖
                  </Button>
                </div>
              </div>
            ) : (
              <p className="text-xs text-muted-foreground">
                未发现同名角色，确认后会新建。
              </p>
            )}
          </div>
        )}
        <ActionError error={fileError ?? error} />
        <DialogFooter>
          {preview !== null && (
            <Button
              variant="outline"
              disabled={pending}
              onClick={() => {
                setPreview(null)
                setDecision(null)
                setFileError(null)
              }}
            >
              重新选择
            </Button>
          )}
          <Button variant="ghost" disabled={pending} onClick={close}>
            取消
          </Button>
          {preview !== null && (
            <Button
              disabled={!canCommit}
              onClick={() =>
                onCommit(preview, decision, () => {
                  setPreview(null)
                  setDecision(null)
                  onClose()
                })
              }
            >
              {pending ? '正在导入…' : '确认导入'}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
