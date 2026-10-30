import { Monitor, Moon, Sun } from 'lucide-react'
import { useTheme } from 'next-themes'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { PageHeader } from '../components/page-parts'

const options = [
  {
    value: 'light',
    label: '浅色',
    description: '明亮、清晰的工作界面',
    icon: Sun,
  },
  { value: 'dark', label: '深色', description: '适合低光环境', icon: Moon },
  {
    value: 'system',
    label: '跟随系统',
    description: '随设备外观自动切换',
    icon: Monitor,
  },
]

export default function ThemePage() {
  const { theme, setTheme, resolvedTheme } = useTheme()
  return (
    <section>
      <PageHeader
        title="主题"
        description="选择适合你的外观。设置即时应用于整个 ORA V2，并保存在此浏览器。"
      />
      <div className="max-w-2xl border-t pt-6">
        <h2 className="mb-4 text-sm font-medium">界面外观</h2>
        <ToggleGroup
          aria-label="界面外观"
          value={[theme ?? 'system']}
          onValueChange={(values) => {
            const next = values[0]
            if (typeof next === 'string') setTheme(next)
          }}
          variant="outline"
          className="grid w-full grid-cols-1 gap-3 sm:grid-cols-3"
        >
          {options.map((option) => (
            <ToggleGroupItem
              key={option.value}
              value={option.value}
              aria-label={option.label}
              className="h-auto min-w-0 flex-col items-start gap-3 p-5"
            >
              <option.icon className="size-5" aria-hidden="true" />
              <span>{option.label}</span>
              <span className="text-xs font-normal text-muted-foreground">
                {option.description}
              </span>
            </ToggleGroupItem>
          ))}
        </ToggleGroup>
        <p role="status" className="mt-6 text-sm text-muted-foreground">
          当前显示：{resolvedTheme === 'dark' ? '深色' : '浅色'}
          {theme === 'system' ? ' · 正在跟随系统' : ''}
        </p>
        <p className="mt-2 text-xs text-muted-foreground">
          此偏好仅作用于当前浏览器，不会更改其他设备。
        </p>
      </div>
    </section>
  )
}
