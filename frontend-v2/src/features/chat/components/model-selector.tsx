import { BotIcon, CheckIcon, SearchIcon } from 'lucide-react'
import { useState } from 'react'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import type { Agent } from '../store'

const item = 'gap-1.5 px-2 py-1.5 text-xs'
const label = 'px-2 py-1.5 text-xs font-normal text-muted-foreground'

/**
 * Picks the agent and its model, as desktop's selector does: the trigger shows the model and
 * reveals the agent's name on hover; picking an agent switches to its first model and keeps the
 * menu open so one of its models can be chosen, and the models can be searched.
 */
export function ModelSelector({
  agents,
  agent,
  model,
  onChoose,
}: {
  agents: Agent[]
  agent: string
  model: string
  onChoose: (agent: string, model: string) => void
}) {
  const [query, setQuery] = useState('')
  const current = agents.find((a) => a.id === agent)
  const models = current?.models ?? []
  const shown = models.filter((m) => m.toLowerCase().includes(query.trim().toLowerCase()))

  return (
    <DropdownMenu onOpenChange={(open) => open || setQuery('')}>
      <DropdownMenuTrigger
        render={
          <Button
            type="button"
            variant="ghost"
            size="sm"
            aria-label={`模型：${current?.name ?? agent} ${model}`}
            className="group/model h-7 gap-1.5 rounded-md px-2 text-xs font-normal text-muted-foreground hover:bg-muted/60 hover:text-foreground"
          />
        }
      >
        <BotIcon className="size-3.5 shrink-0" />
        {/* The agent's name grows in on hover through a 0fr → 1fr grid, as in desktop. */}
        <span className="grid grid-cols-[0fr] opacity-0 transition-all duration-200 group-hover/model:grid-cols-[1fr] group-hover/model:opacity-100 group-aria-expanded/model:grid-cols-[1fr] group-aria-expanded/model:opacity-100">
          <span className="min-w-0 overflow-hidden whitespace-nowrap">{current?.name}</span>
        </span>
        <span className="whitespace-nowrap">{model}</span>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" side="top" className="max-h-[min(24rem,var(--available-height))] w-60">
        <DropdownMenuGroup>
          <DropdownMenuLabel className={label}>Agent</DropdownMenuLabel>
          {agents.map((a) => (
            <DropdownMenuItem key={a.id} className={item} closeOnClick={false} onClick={() => a.id !== agent && onChoose(a.id, a.models[0])}>
              <BotIcon className="size-3.5 text-muted-foreground" />
              {a.name}
              {a.id === agent && <CheckIcon className="ml-auto size-4" />}
            </DropdownMenuItem>
          ))}
        </DropdownMenuGroup>
        <DropdownMenuGroup>
          <DropdownMenuLabel className={label}>模型</DropdownMenuLabel>
          {/* Only the models are searched: a query should never hide which agents there are. */}
          <div className="relative px-0.5 pb-1">
            <SearchIcon className="pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-muted-foreground" />
            <Input
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="搜索模型"
              aria-label="搜索模型"
              className="h-7 border-transparent bg-muted/50 pl-8 text-xs shadow-none md:text-xs"
              // Out of the tab order so opening the menu does not drop focus into the search box.
              tabIndex={-1}
              // The menu's typeahead and arrow keys listen on the popup; keep the keystrokes here.
              onKeyDown={(e) => e.stopPropagation()}
              onClick={(e) => e.stopPropagation()}
            />
          </div>
          {shown.length === 0 ? (
            <p className="px-2 py-4 text-center text-xs text-muted-foreground">{models.length ? '未找到匹配的模型' : '该 Agent 未提供可选模型'}</p>
          ) : (
            shown.map((m) => (
              <DropdownMenuItem key={m} className={item} onClick={() => onChoose(agent, m)}>
                {m}
                {m === model && <CheckIcon className="ml-auto size-4" />}
              </DropdownMenuItem>
            ))
          )}
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
