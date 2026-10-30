import { BoxIcon } from 'lucide-react'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'

/**
 * Says that a chat runs in a sandbox of its own: the backend clones the project's repository
 * into a new directory for every conversation, and nothing there is written back.
 */
export function SandboxLabel() {
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
        <BoxIcon className="size-3.5" />
        沙盒
      </TooltipTrigger>
      <TooltipContent>每个对话在独立的沙盒中运行，代码从项目仓库 clone 而来，改动不会写回</TooltipContent>
    </Tooltip>
  )
}
