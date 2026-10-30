import type { ReactNode } from 'react'
import { LoaderCircle, Search, SearchX } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from '@/components/ui/input-group'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'

export function PageHeader({
  title,
  description,
  count,
  actions,
}: {
  title: string
  description: string
  count?: number
  actions?: ReactNode
}) {
  return (
    <header className="settings-page-header">
      <div className="min-w-0">
        <div className="flex items-center gap-3">
          <h1 className="text-2xl font-semibold tracking-tight">{title}</h1>
          {count !== undefined && <Badge variant="secondary">{count}</Badge>}
        </div>
        <p className="mt-2 text-sm leading-relaxed text-muted-foreground">
          {description}
        </p>
      </div>
      {actions && (
        <div className="flex shrink-0 flex-wrap gap-2">{actions}</div>
      )}
    </header>
  )
}
export function DataState({
  pending,
  error,
  retry,
  children,
}: {
  pending: boolean
  error: unknown
  retry: () => void
  children: ReactNode
}) {
  if (pending)
    return (
      <div
        role="status"
        className="flex min-h-64 items-center justify-center gap-2 text-sm text-muted-foreground"
      >
        <LoaderCircle
          className="size-4 animate-spin motion-reduce:animate-none"
          aria-hidden="true"
        />
        正在加载…
      </div>
    )
  if (error)
    return (
      <Empty className="min-h-64">
        <EmptyHeader>
          <EmptyTitle>暂时无法加载</EmptyTitle>
          <EmptyDescription>
            <span role="alert">
              {error instanceof Error ? error.message : '读取失败，请重试。'}
            </span>
          </EmptyDescription>
        </EmptyHeader>
        <EmptyContent>
          <Button variant="outline" onClick={retry}>
            重试
          </Button>
        </EmptyContent>
      </Empty>
    )
  return <>{children}</>
}
export function ActionError({ error }: { error: unknown }) {
  return error ? (
    <p
      role="alert"
      className="rounded-lg border border-destructive/30 p-3 text-sm text-destructive"
    >
      {error instanceof Error ? error.message : '操作失败，请重试。'}
    </p>
  ) : null
}
export function SearchInput({
  value,
  onChange,
  label,
}: {
  value: string
  onChange: (value: string) => void
  label: string
}) {
  return (
    <InputGroup className="w-full sm:max-w-72">
      <InputGroupAddon>
        <Search aria-hidden="true" />
      </InputGroupAddon>
      <InputGroupInput
        aria-label={label}
        placeholder={label + '…'}
        value={value}
        onChange={(event) => onChange(event.target.value)}
      />
    </InputGroup>
  )
}
export function Filter({
  label,
  value,
  onChange,
  options,
}: {
  label: string
  value: string
  onChange: (value: string) => void
  options: readonly (readonly [string, string])[]
}) {
  return (
    <NativeSelect
      aria-label={label}
      value={value}
      onChange={(event) => onChange(event.target.value)}
    >
      {options.map(([id, title]) => (
        <NativeSelectOption key={id} value={id}>
          {title}
        </NativeSelectOption>
      ))}
    </NativeSelect>
  )
}
export function NoResults({
  filtered,
  onClear,
  children,
}: {
  filtered: boolean
  onClear: () => void
  children?: ReactNode
}) {
  return (
    <Empty className="min-h-64">
      <EmptyHeader>
        <EmptyMedia variant="icon">
          <SearchX />
        </EmptyMedia>
        <EmptyTitle>
          {filtered ? '没有匹配的结果' : '这里还没有内容'}
        </EmptyTitle>
        <EmptyDescription>
          {filtered
            ? '试试其他关键词，或清除筛选条件。'
            : '添加第一项内容，开始管理你的工作空间。'}
        </EmptyDescription>
      </EmptyHeader>
      <EmptyContent>
        {filtered ? (
          <Button variant="outline" onClick={onClear}>
            清除筛选
          </Button>
        ) : (
          children
        )}
      </EmptyContent>
    </Empty>
  )
}
