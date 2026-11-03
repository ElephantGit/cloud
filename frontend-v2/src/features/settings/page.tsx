import { useEffect, useRef } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { NavLink, Outlet, useLocation } from 'react-router'
import { Settings } from 'lucide-react'
import { Toaster } from '@/components/ui/sonner'
import { cn } from '@/lib/utils'
import { settingsPages } from './navigation'
import { useSettingsData } from './data/hooks'
import './settings.css'

export default function SettingsPage() {
  const location = useLocation()
  const { repository } = useSettingsData()
  const client = useQueryClient()
  const themePage = location.pathname === '/settings/theme'
  const navigation = useRef<HTMLElement>(null)
  useEffect(() => {
    const keepCurrentVisible = () =>
      navigation.current
        ?.querySelector('[aria-current="page"]')
        ?.scrollIntoView({ block: 'nearest', inline: 'center' })
    keepCurrentVisible()
    const observer = new ResizeObserver(keepCurrentVisible)
    if (navigation.current) observer.observe(navigation.current)
    return () => observer.disconnect()
  }, [location.pathname])
  useEffect(() => {
    const refresh = (event: StorageEvent) => {
      if (event.key === repository.key || event.key === null)
        void client.invalidateQueries({
          queryKey: ['settings-demo', repository.key],
        })
    }
    window.addEventListener('storage', refresh)
    return () => window.removeEventListener('storage', refresh)
  }, [client, repository])
  return (
    <div className="settings-layout">
      <aside className="settings-sidebar">
        <div className="flex items-center gap-2 px-3 pb-5 pt-1 text-base font-semibold">
          <Settings
            className="size-4 text-muted-foreground"
            aria-hidden="true"
          />
          设置
        </div>
        <nav
          ref={navigation}
          aria-label="设置导航"
          className="settings-navigation"
        >
          {settingsPages.map((page) => (
            <NavLink
              key={page.path}
              to={page.path}
              className={({ isActive }) =>
                cn('settings-nav-link', isActive && 'settings-nav-active')
              }
            >
              <page.icon className="size-4 shrink-0" aria-hidden="true" />
              {page.title}
            </NavLink>
          ))}
        </nav>
        <div className="settings-sidebar-note">个人偏好与工作空间资源</div>
      </aside>
      <div className="settings-main">
        <div className="settings-context-bar">
          <span>
            设置 <span className="px-2 text-muted-foreground/60">/</span>{' '}
            {
              settingsPages.find((p) =>
                location.pathname.endsWith('/' + p.path),
              )?.title
            }
          </span>
          <span className="text-xs">
            {themePage
              ? '浏览器偏好'
              : location.pathname === '/settings/plugins'
                ? '工作空间资源'
                : location.pathname === '/settings/skills'
                  ? '服务端技能目录'
                  : location.pathname === '/settings/model-connections'
                    ? '个人设置'
                    : location.pathname === '/settings/roles'
                      ? '本机角色'
                      : '工作空间成员'}
          </span>
        </div>
        <div className="settings-content">
          <Outlet />
        </div>
      </div>
      <Toaster position="bottom-right" closeButton />
    </div>
  )
}
