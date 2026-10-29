import { NavLink } from 'react-router'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'
import { UserMenu } from './auth/user-menu'
import { features } from './features'

/** The narrow icon column on the far left: one icon per feature page. */
export function NavRail() {
  return (
    <nav aria-label="主导航" className="flex w-13 shrink-0 flex-col items-center gap-1 border-r bg-muted/40 py-3">
      <div className="mb-3 flex size-8 items-center justify-center rounded-lg bg-primary text-sm font-semibold text-primary-foreground">O</div>
      {features.map((f) => (
        <Tooltip key={f.path}>
          <TooltipTrigger
            render={
              <NavLink
                to={`/${f.path}`}
                aria-label={f.title}
                className={({ isActive }) =>
                  cn(
                    'flex size-9 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-accent hover:text-foreground',
                    isActive && 'bg-background text-foreground shadow-sm ring-1 ring-border',
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
