import { Button } from '@/components/ui/button'
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'

/** Asks for the branch a new worktree of `projectName` checks out. */
export function NewWorktreeDialog({
  open,
  onOpenChange,
  projectName,
  onCreate,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  projectName: string
  onCreate: (branch: string) => void
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <form
          className="contents"
          onSubmit={(e) => {
            e.preventDefault()
            const branch = String(new FormData(e.currentTarget).get('branch') ?? '').trim()
            if (branch) onCreate(branch)
          }}
        >
          <DialogHeader>
            <DialogTitle>新建 worktree</DialogTitle>
            <DialogDescription>Agent 会在 {projectName} 的独立 worktree 里工作，不影响主目录。</DialogDescription>
          </DialogHeader>
          <Input name="branch" aria-label="分支名" placeholder="feat/my-change" autoFocus required pattern="[A-Za-z0-9._/\-]+" />
          <DialogFooter>
            <DialogClose render={<Button variant="outline" type="button" />}>取消</DialogClose>
            <Button type="submit">创建</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
