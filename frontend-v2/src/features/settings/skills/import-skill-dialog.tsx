import { useId, useState } from 'react'
import { FileText, Upload, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { ActionError } from '../components/page-parts'
import type { ImportSkillInput, Skill } from '@/lib/skills-api/generated'
import { isSkillNameConflict, skillError } from './use-skills'

const MAX_FILE_BYTES = 1 << 20

/**
 * Imports one SKILL.md. The server owns front-matter validation and name conflicts; the dialog
 * only asks for an explicit overwrite after the first attempt returns a conflict.
 */
export function ImportSkillDialog({
  onClose,
  onImport,
  onImported,
}: {
  onClose: () => void
  onImport: (input: ImportSkillInput) => Promise<Skill>
  onImported: (name: string, overwritten: boolean) => void
}) {
  const inputId = useId()
  const [file, setFile] = useState<File>()
  const [content, setContent] = useState('')
  const [conflict, setConflict] = useState(false)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState<unknown>()

  const choose = async (next?: File) => {
    setError(undefined)
    setConflict(false)
    if (!next) return
    if (!/\.(md|markdown)$/i.test(next.name)) {
      setError(new Error('请选择 .md 格式的 SKILL.md 文件。'))
      return
    }
    if (next.size > MAX_FILE_BYTES) {
      setError(new Error('SKILL.md 不能超过 1 MiB。'))
      return
    }
    try {
      const text = await next.text()
      setFile(next)
      setContent(text)
    } catch {
      setError(new Error('无法读取该文件，请重新选择。'))
    }
  }

  const submit = async (overwrite: boolean) => {
    if (!file || !content) return
    setPending(true)
    setError(undefined)
    try {
      const imported = await onImport({
        fileName: file.name,
        content,
        overwrite,
      })
      onImported(imported.name, overwrite)
    } catch (cause) {
      if (!overwrite && isSkillNameConflict(cause)) {
        setConflict(true)
      } else {
        setError(skillError(cause))
      }
    } finally {
      setPending(false)
    }
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !pending) onClose()
      }}
    >
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>导入技能</DialogTitle>
          <DialogDescription>
            选择一个带 YAML front matter 的 SKILL.md。导入后，新开启的 Agent
            会话会自动发现它。
          </DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-4">
          {file ? (
            <div className="flex items-start gap-3 rounded-lg border p-4">
              <FileText className="mt-0.5 size-5 shrink-0 text-muted-foreground" />
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm font-medium">{file.name}</p>
                <p className="mt-1 text-xs text-muted-foreground">
                  {Math.max(1, Math.ceil(file.size / 1024))} KiB · Markdown
                </p>
              </div>
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                aria-label="重新选择文件"
                disabled={pending}
                onClick={() => {
                  setFile(undefined)
                  setContent('')
                  setConflict(false)
                  setError(undefined)
                }}
              >
                <X />
              </Button>
            </div>
          ) : (
            <label
              htmlFor={inputId}
              className="flex min-h-40 cursor-pointer flex-col items-center justify-center gap-3 rounded-xl border border-dashed p-6 text-center transition-colors hover:bg-muted/40 focus-within:ring-2 focus-within:ring-ring"
            >
              <span className="flex size-10 items-center justify-center rounded-full bg-muted">
                <Upload className="size-5" />
              </span>
              <span>
                <span className="block text-sm font-medium">选择 SKILL.md</span>
                <span className="mt-1 block text-xs text-muted-foreground">
                  文件最大 1 MiB，名称与简介写在 YAML front matter 中
                </span>
              </span>
            </label>
          )}
          <input
            id={inputId}
            className="sr-only"
            type="file"
            accept=".md,.markdown,text/markdown"
            disabled={pending}
            onChange={(event) => {
              void choose(event.currentTarget.files?.item(0) ?? undefined)
              event.currentTarget.value = ''
            }}
          />
          {conflict && (
            <div
              role="alert"
              className="rounded-lg border border-warning/30 bg-warning/10 p-3 text-sm"
            >
              <p className="font-medium">已有同名技能</p>
              <p className="mt-1 text-muted-foreground">
                覆盖会用这份 SKILL.md 更新现有技能，并保留它的技能 ID。
              </p>
            </div>
          )}
          <ActionError error={error} />
          <p className="text-xs leading-relaxed text-muted-foreground">
            当前仅支持单个 Markdown 文件；文件夹、归档和附带脚本的 Skill
            包暂不支持。
          </p>
        </div>
        <DialogFooter>
          <Button
            type="button"
            variant="ghost"
            disabled={pending}
            onClick={onClose}
          >
            取消
          </Button>
          {conflict ? (
            <Button
              type="button"
              disabled={pending}
              onClick={() => void submit(true)}
            >
              {pending ? '导入中…' : '覆盖并导入'}
            </Button>
          ) : (
            <Button
              type="button"
              disabled={!file || pending}
              onClick={() => void submit(false)}
            >
              {pending ? '导入中…' : '导入'}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
