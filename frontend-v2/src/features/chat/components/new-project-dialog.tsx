import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'

/** Asks for the folder a new project lives in, as desktop does; the project is named after it. */
export function NewProjectDialog({
  open,
  onOpenChange,
  onCreate,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  onCreate: (path: string) => void
}) {
  const [path, setPath] = useState('')
  const name = path.trim().replace(/[\\/]+$/, '').split(/[\\/]/).at(-1)
  return (
    <Dialog open={open} onOpenChange={(o) => (onOpenChange(o), o || setPath(''))}>
      <DialogContent>
        <form
          className="contents"
          onSubmit={(e) => {
            e.preventDefault()
            if (!path.trim()) return
            onCreate(path.trim())
            setPath('')
          }}
        >
          <DialogHeader>
            <DialogTitle>添加项目</DialogTitle>
            <DialogDescription>填写项目在云端机器上的路径，项目名取最后一级目录名。</DialogDescription>
          </DialogHeader>
          <div className="flex flex-col gap-1.5">
            <Input
              name="path"
              aria-label="项目文件夹"
              placeholder="/workspace/my-project"
              autoFocus
              required
              value={path}
              onChange={(e) => setPath(e.target.value)}
              className="font-mono"
            />
            {name && <p className="text-xs text-muted-foreground">项目名：{name}</p>}
          </div>
          <DialogFooter>
            <DialogClose render={<Button variant="outline" type="button" />}>取消</DialogClose>
            <Button type="submit">添加项目</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
