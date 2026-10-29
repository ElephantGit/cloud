import { useMutation, useQueryClient } from '@tanstack/react-query'
import { LogOut } from 'lucide-react'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { logout } from '@/lib/auth-api'
import { useUser } from './user'

/** The bottom of the icon rail: who is signed in, and sign out. */
export function UserMenu() {
  const user = useUser()
  const queryClient = useQueryClient()
  // Dropping every cached query makes the session gate ask /api/v1/me again and show the login.
  const signOut = useMutation({ mutationFn: logout, onSuccess: () => queryClient.resetQueries() })

  return (
    <div className="mt-auto flex flex-col items-center gap-2">
      <Tooltip>
        <TooltipTrigger
          render={
            <button
              type="button"
              aria-label="退出登录"
              disabled={signOut.isPending}
              onClick={() => signOut.mutate()}
              className="flex size-9 items-center justify-center rounded-lg text-muted-foreground hover:bg-accent hover:text-foreground"
            />
          }
        >
          <LogOut className="size-[18px]" />
        </TooltipTrigger>
        <TooltipContent side="right">退出登录</TooltipContent>
      </Tooltip>
      <Tooltip>
        <TooltipTrigger
          render={<div role="img" aria-label={user.displayName} className="flex size-8 items-center justify-center rounded-full bg-secondary text-xs font-medium" />}
        >
          {user.displayName.slice(0, 1)}
        </TooltipTrigger>
        <TooltipContent side="right">{user.displayName}</TooltipContent>
      </Tooltip>
    </div>
  )
}
