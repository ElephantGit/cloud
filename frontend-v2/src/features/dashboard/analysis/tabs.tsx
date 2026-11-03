import { createContext, type ReactNode, useContext, useId, useState } from 'react'

const Context = createContext({ value: '', select: (_value: string) => {}, id: '' })
/** Local read-only inspector tabs; state belongs to this event's inspector. */
export function Tabs(
  { defaultValue, children, className }: {
    defaultValue: string
    children: ReactNode
    className?: string
  },
) {
  const [value, select] = useState(defaultValue)
  const id = useId()
  return (
    <Context.Provider value={{ value, select, id }}>
      <div className={className}>{children}</div>
    </Context.Provider>
  )
}
export function TabsList({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <div
      role='tablist'
      aria-label='事件详情页签'
      className={`flex flex-wrap gap-1 ${className ?? ''}`}
    >
      {children}
    </div>
  )
}
export function TabsTrigger({ value, children }: { value: string; children: ReactNode }) {
  const tabs = useContext(Context)
  return (
    <button
      type='button'
      role='tab'
      id={`${tabs.id}-${value}`}
      aria-controls={`${tabs.id}-${value}-panel`}
      aria-selected={tabs.value === value}
      className='rounded px-3 py-2 text-xs aria-selected:bg-muted'
      onClick={() => tabs.select(value)}
    >
      {children}
    </button>
  )
}
export function TabsContent({ value, children }: { value: string; children: ReactNode }) {
  const tabs = useContext(Context)
  return tabs.value === value
    ? (
      <section
        role='tabpanel'
        id={`${tabs.id}-${value}-panel`}
        aria-labelledby={`${tabs.id}-${value}`}
        className='pt-3'
      >
        {children}
      </section>
    )
    : null
}
