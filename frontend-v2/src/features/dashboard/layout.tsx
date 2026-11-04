import { NavLink, Outlet } from 'react-router'

/** Both demo team progress and real session analysis live in #882's dashboard. */
export default function DashboardLayout() {
  return (
    <div className='h-full min-h-0 overflow-auto'>
      <nav aria-label='仪表盘视图' className='flex gap-2 border-b bg-background p-3'>
        <NavLink
          end
          to='/dashboard'
          className='rounded px-3 py-2 text-sm aria-[current=page]:bg-muted'
        >
          团队概览
        </NavLink>
        <NavLink
          to='/dashboard/sessions'
          className='rounded px-3 py-2 text-sm aria-[current=page]:bg-muted'
        >
          会话分析
        </NavLink>
      </nav>
      <Outlet />
    </div>
  )
}
