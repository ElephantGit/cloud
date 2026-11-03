import { NavLink } from 'react-router'
import { OraMark } from '@/components/ora-mark'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'
import { UserMenu } from './auth/user-menu'
import { features } from './features'

/** The narrow icon column on the far left: one icon per feature page. */
export function NavRail() {
  return (
    <nav aria-label="主导航" className="flex w-13 shrink-0 flex-col items-center gap-1.5 border-r border-sidebar-border bg-sidebar py-3">
      <OraMark className="mb-2.5 size-8 shadow-sm dark:ring-1 dark:ring-white/10" />
      {features.map((f) => (
        <Tooltip key={f.path}>
          <TooltipTrigger
            render={
              <NavLink
                to={`/${f.path}`}
                aria-label={f.title}
                className={({ isActive }) =>
                  cn(
                    'flex size-9 items-center justify-center rounded-lg text-muted-foreground transition-colors duration-150 outline-none hover:bg-sidebar-accent hover:text-foreground focus-visible:ring-3 focus-visible:ring-ring/50',
                    isActive && 'bg-brand-soft text-brand hover:bg-brand-soft hover:text-brand',
                  )
                }
              />
            }
          >
            <f.icon className="size-[18px]" />
          </TooltipTrigger>
          <TooltipContent side="right">{f.title}</TooltipContent>
        </Tooltip>
      ))}
      <UserMenu />
    </nav>
  )
}
