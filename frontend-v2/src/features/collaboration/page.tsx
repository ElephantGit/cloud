import { useQuery } from '@tanstack/react-query'
import { Link, NavLink, Outlet, useLocation, useSearchParams } from 'react-router'
import { ClipboardCheck, ClipboardList, Inbox, Plus, UserRound, Users } from 'lucide-react'
import { useUser } from '@/app/auth/user'
import { buttonVariants } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { listSquads } from './api/generated'
import { filterTasks, TASK_VIEWS, viewOf, type TaskView } from './task-views'
import { usePendingMentions, useTaskList, useWorkspace } from './workspace'
import './collaboration.css'

const VIEW_ICONS: Record<TaskView, typeof Inbox> = { mine: UserRound, todo: Inbox, all: ClipboardList }

/**
 * 协作: the team's shared work. The sidebar scopes the one task list (mine, waiting on me,
 * everything) and lists the squads; the outlet shows the board, a task, or squads. The scopes
 * differ only by query, so they are plain links marked current by hand: NavLink would mark them all.
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
  const squads = useQuery({ queryKey: ['squads', tenantId], queryFn: () => listSquads(tenantId), enabled: !!tenantId })
  const onList = location.pathname === '/collaboration' || location.pathname === '/collaboration/'
  const view = viewOf(params.get('view'))
  const squadId = params.get('squad')
  const items = tasks.data?.items ?? []
  const liveSquads = squads.data?.items ?? []
  return <div className="collaboration-feature mc-layout">
    <aside className="mc-sidebar" aria-label="协作导航">
      <div className="mc-sidebar-head">
        <h1>协作</h1>
        <span>团队共享</span>
      </div>
      <NavLink className={cn(buttonVariants(), 'mc-new-task')} to={`/collaboration?view=${onList ? view : 'all'}${squadId ? `&squad=${squadId}` : ''}&new=1`}>
        <Plus data-icon="inline-start" />新建任务
      </NavLink>
      <nav className="mc-nav" aria-label="任务视图">
        <span className="mc-nav-label">任务</span>
        {TASK_VIEWS.map(v => {
          const Icon = VIEW_ICONS[v.id]
          const active = onList && !squadId && view === v.id
          return <Link key={v.id} to={`/collaboration?view=${v.id}`} title={v.hint} aria-current={active ? 'page' : undefined} className={cn('mc-nav-link', active && 'active')}>
            <Icon size={15} aria-hidden="true" />
            <span>{v.label}</span>
            <b>{tasks.data ? filterTasks(items, v.id, me.id, null, mentioned).length : ''}</b>
          </Link>
        })}
      </nav>
      <nav className="mc-nav" aria-label="小队">
        <span className="mc-nav-label">小队</span>
        {liveSquads.map(s => {
          const active = onList && squadId === s.id
          return <Link key={s.id} to={`/collaboration?view=all&squad=${s.id}`} aria-current={active ? 'page' : undefined} className={cn('mc-nav-link', active && 'active')}>
            <ClipboardCheck size={15} aria-hidden="true" />
            <span>{s.name}</span>
            <b>{tasks.data ? filterTasks(items, 'all', me.id, s.id).length : ''}</b>
          </Link>
        })}
        <NavLink to="/collaboration/squads" className={({ isActive }) => cn('mc-nav-link', isActive && 'active')}>
          <Users size={15} aria-hidden="true" />
          <span>管理小队</span>
        </NavLink>
      </nav>
      <p className="mc-sidebar-note">任务和运行全团队可见；指派给小队不会自动分派。</p>
    </aside>
    <main className="mc-main">
      <Outlet />
    </main>
  </div>
}
