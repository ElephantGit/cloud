import { Outlet } from 'react-router'

/** A single scroll surface for the dashboard and session details. */
export default function DashboardLayout() {
  return <div className='h-full min-h-0 overflow-auto'><Outlet /></div>
}
