import { Link, NavLink, Outlet, useLocation, useSearchParams } from 'react-router'
import { ClipboardList, Inbox, Plus, UserRound } from 'lucide-react'
import { useUser } from '@/app/auth/user'
import { buttonVariants } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { filterTasks, TASK_VIEWS, viewOf, type TaskView } from './task-views'
import { useMySquadTasks, usePendingMentions, useTaskList, useWorkspace } from './workspace'
import '@/components/panel/panel.css'
import './collaboration.css'

const VIEW_ICONS: Record<TaskView, typeof Inbox> = { mine: UserRound, todo: Inbox, all: ClipboardList }

/**
 * 协作: the team's shared work. The sidebar scopes the one task list (mine, waiting on me, or
 * everything); the outlet shows the board or a task. Squads are configured under 设置 / 小队. The
 * scopes differ only by query, so they are plain links marked current by hand: NavLink would mark them all.
 * Everything here is the workspace's; a person's private conversations stay on the chat page.
 */
export default function CollaborationLayout() {
  const me = useUser()
  const { tenantId } = useWorkspace()
  const location = useLocation()
  const [params] = useSearchParams()
  const tasks = useTaskList(tenantId)
  const mentions = usePendingMentions(tenantId)
  const mentioned = new Set(mentions.keys())
  const squadTasks = useMySquadTasks(tenantId)
  const onList = location.pathname === '/collaboration' || location.pathname === '/collaboration/'
  const view = viewOf(params.get('view'))
  const squadId = onList ? params.get('squad') : null
  const squadQuery = squadId ? `&squad=${squadId}` : ''
  const items = tasks.data?.items ?? []
  return <div className="mc-scope mc-layout">
    <aside className="mc-sidebar" aria-label="协作导航">
      <div className="mc-sidebar-head">
        <h1>协作</h1>
        <span>团队共享</span>
      </div>
      <NavLink className={cn(buttonVariants(), 'mc-new-task')} to={`/collaboration?view=${onList ? view : 'all'}${squadQuery}&new=1`}>
        <Plus data-icon="inline-start" />新建任务
      </NavLink>
      <nav className="mc-nav" aria-label="任务视图">
        <span className="mc-nav-label">任务</span>
        {TASK_VIEWS.map(v => {
          const Icon = VIEW_ICONS[v.id]
          const active = onList && view === v.id
          return <Link key={v.id} to={`/collaboration?view=${v.id}${squadQuery}`} title={v.hint} aria-current={active ? 'page' : undefined} className={cn('mc-nav-link', active && 'active')}>
            <Icon size={15} aria-hidden="true" />
            <span>{v.label}</span>
            <b>{tasks.data ? filterTasks(items, v.id, me.id, mentioned, squadTasks, squadId).length : ''}</b>
          </Link>
        })}
      </nav>
      <p className="mc-sidebar-note">任务和运行全团队可见；指派给小队的任务，每个队员都会收到。</p>
    </aside>
    <main className="mc-main">
      <Outlet />
    </main>
  </div>
}
