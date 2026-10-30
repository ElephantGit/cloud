import { GitBranchIcon } from 'lucide-react'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'

/**
 * Says that a chat always works in a worktree of its own, as desktop's input marks worktree mode:
 * the backend makes a new directory from the project's repository for every conversation.
 */
export function WorktreeLabel() {
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <span
            tabIndex={0}
            className="inline-flex h-6 shrink-0 cursor-default items-center gap-1 rounded-md bg-blue-500/10 px-1.5 text-xs text-blue-600 outline-none focus-visible:ring-2 focus-visible:ring-ring/50 dark:text-blue-400"
          />
        }
      >
        <GitBranchIcon className="size-3.5" />
        Worktree
      </TooltipTrigger>
      <TooltipContent>每个对话都在新建的 worktree 里工作</TooltipContent>
    </Tooltip>
  )
}
