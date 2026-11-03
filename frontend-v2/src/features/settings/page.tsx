import { useEffect, useRef, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { NavLink, Outlet, useLocation } from 'react-router'
import { FlaskConical, RotateCcw, Settings } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Toaster } from '@/components/ui/sonner'
import { cn } from '@/lib/utils'
import { settingsPages } from './navigation'
import { useSettingsData } from './data/hooks'
import { ConfirmDialog } from './components/confirm-dialog'
import './settings.css'

export default function SettingsPage() {
  const location = useLocation()
  const [confirming, setConfirming] = useState(false)
  const { repository, queryKey, reset } = useSettingsData()
  const client = useQueryClient()
  const themePage = location.pathname === '/settings/theme'
  // Plugins, skills and members are real server resources. Roles and model connections remain
  // browser-local.
  const localDemoPage = [
    '/settings/model-connections',
    '/settings/roles',
  ].includes(location.pathname)
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
                  ? '个人技能目录'
                  : location.pathname === '/settings/model-connections'
                    ? '演示工作空间'
                    : location.pathname === '/settings/roles'
                      ? '本机角色'
                      : '工作空间成员'}
          </span>
        </div>
        <div className="settings-content">
          {localDemoPage && (
            <div className="settings-demo-notice">
              <FlaskConical
                className="mt-0.5 size-4 shrink-0"
                aria-hidden="true"
              />
              <div className="min-w-0 flex-1">
                <p className="font-medium">演示数据，仅保存在此浏览器</p>
                <p className="mt-1 text-xs leading-relaxed text-muted-foreground">
                  操作不会调用模型或修改在线资源。每个登录用户独立保存。
                </p>
              </div>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => {
                  reset.reset()
                  setConfirming(true)
                }}
              >
                <RotateCcw data-icon="inline-start" />
                重置演示
              </Button>
            </div>
          )}
          <Outlet />
        </div>
      </div>
      {confirming && (
        <ConfirmDialog
          title="重置演示数据？"
          description="将清除当前用户在此浏览器中的模型连接和角色修改，恢复初始示例。主题、工作空间插件与技能不受影响。"
          confirmLabel="重置演示"
          pending={reset.isPending}
          error={reset.error}
          onClose={() => setConfirming(false)}
          onConfirm={() =>
            reset.mutate(undefined, {
              onSuccess: () => {
                void client.invalidateQueries({ queryKey })
                setConfirming(false)
              },
            })
          }
        />
      )}
      <Toaster position="bottom-right" closeButton />
    </div>
  )
}
