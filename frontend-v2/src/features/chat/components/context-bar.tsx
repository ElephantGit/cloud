import { CheckIcon, ChevronDownIcon, ChevronRightIcon, CloudIcon, FolderIcon, FolderPlusIcon, GitBranchIcon, GitBranchPlusIcon, LaptopIcon, PencilIcon } from 'lucide-react'
import { type ReactNode, useState } from 'react'
import { Button } from '@/components/ui/button'
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList, CommandSeparator, CommandShortcut } from '@/components/ui/command'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { backendSupports, type NewChat, type NewChatWorkspace, type Project } from '../store'
import { NewProjectDialog } from './new-project-dialog'
import { NewWorktreeDialog } from './new-worktree-dialog'
import { WorktreeLabel } from './worktree-label'

const tab =
  'group/context h-6 min-w-0 gap-1 rounded-md px-1.5 text-xs font-normal text-muted-foreground hover:bg-muted/60 hover:text-foreground data-popup-open:bg-muted/60 data-popup-open:text-foreground'
const item = 'gap-1.5 px-2 py-1.5 text-xs'
const icon = 'size-3.5 text-muted-foreground'
const heading = '**:[[cmdk-group-heading]]:font-normal'

function TabChevron() {
  return <ChevronDownIcon className="size-3 opacity-50 transition-transform group-data-popup-open/context:rotate-180" aria-hidden />
}

/** A context tab whose menu searches its items, as desktop's project and branch tabs do. */
function SearchTab({ label, trigger, placeholder, empty, children }: { label: string; trigger: ReactNode; placeholder: string; empty: string; children: (close: () => void) => ReactNode }) {
  const [open, setOpen] = useState(false)
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger render={<Button type="button" variant="ghost" size="sm" className={tab} aria-label={label} />}>
        {trigger}
        <TabChevron />
      </PopoverTrigger>
      <PopoverContent align="start" side="top" className="w-60 p-0">
        <Command>
          <CommandInput placeholder={placeholder} aria-label={placeholder} className="text-xs" />
          <CommandList>
            <CommandEmpty className="py-4 text-xs">{empty}</CommandEmpty>
            {children(() => setOpen(false))}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  )
}

/**
 * The row above a new chat's input, as in desktop: which project it starts in, whether in the
 * main checkout or a worktree, and where it runs. Only the cloud runs agents from the web, and it
 * gives every conversation a worktree of its own: then a label says so instead of a workspace tab.
 */
export function ContextBar({
  value,
  projects,
  branches,
  onChange,
  onCreateProject,
}: {
  value: NewChat
  /** Projects a chat can start in. */
  projects: Project[]
  /** The worktree branches the chosen project already has. */
  branches: string[]
  onChange: (patch: Partial<NewChat>) => void
  /** Adds a project from a folder; the menu offers it only when given. */
  onCreateProject?: (path: string) => void
}) {
  const [dialog, setDialog] = useState<'project' | 'worktree'>()
  const project = projects.find((p) => p.id === value.projectId)
  const isolated = value.workspace.kind === 'isolated' ? value.workspace.branch : undefined
  const pending = value.workspace.kind === 'new-worktree'
  const pick = (workspace: NewChatWorkspace) => onChange({ workspace })
  const shown = isolated && !branches.includes(isolated) ? [...branches, isolated] : branches

  return (
    <div className="flex min-w-0 flex-1 items-center gap-1">
      <SearchTab
        label="选择项目"
        placeholder="搜索项目"
        empty="未找到项目。"
        trigger={
          <>
            <FolderIcon className="size-3.5" />
            <span className="max-w-28 min-w-0 truncate sm:max-w-40">{project?.name ?? '选择项目'}</span>
          </>
        }
      >
        {(close) => (
          <>
            <CommandGroup className={heading}>
              {projects.map((p) => (
                <CommandItem
                  key={p.id}
                  value={`${p.name} ${p.path ?? ''}`}
                  data-checked={p.id === value.projectId}
                  className={item}
                  onSelect={() => (onChange({ projectId: p.id, workspace: { kind: 'main' } }), close())}
                >
                  <FolderIcon className={icon} />
                  <span className="truncate">{p.name}</span>
                </CommandItem>
              ))}
            </CommandGroup>
            {onCreateProject && (
              <>
                <CommandSeparator />
                <CommandGroup>
                  <CommandItem value="新建项目" className={item} onSelect={() => (close(), setDialog('project'))}>
                    <FolderPlusIcon className={icon} />
                    新建项目
                    <CommandShortcut>
                      <ChevronRightIcon className="size-3.5" />
                    </CommandShortcut>
                  </CommandItem>
                </CommandGroup>
              </>
            )}
          </>
        )}
      </SearchTab>

      {!backendSupports.chooseWorkspace && <WorktreeLabel />}
      {backendSupports.chooseWorkspace && (
      <SearchTab
        label="选择工作目录"
        placeholder="搜索分支"
        empty="未找到分支。"
        trigger={
          <>
            {pending ? <GitBranchPlusIcon className="size-3.5" /> : isolated ? <GitBranchIcon className="size-3.5" /> : <FolderIcon className="size-3.5" />}
            <span className={isolated ? 'max-w-28 min-w-0 truncate font-mono sm:max-w-40' : 'whitespace-nowrap'}>{pending ? '新 worktree' : (isolated ?? '主目录')}</span>
          </>
        }
      >
        {(close) => (
          <>
            <CommandGroup className={heading}>
              <CommandItem value="主目录" data-checked={value.workspace.kind === 'main'} className={item} onSelect={() => (pick({ kind: 'main' }), close())}>
                <FolderIcon className={icon} />
                主目录
              </CommandItem>
            </CommandGroup>
            {shown.length > 0 && (
              <CommandGroup heading="Worktree" className={heading}>
                {shown.map((branch) => (
                  <CommandItem
                    key={branch}
                    value={branch}
                    data-checked={branch === isolated}
                    className={item}
                    onSelect={() => (pick({ kind: 'isolated', branch }), close())}
                  >
                    <GitBranchIcon className={icon} />
                    <span className="truncate font-mono">{branch}</span>
                  </CommandItem>
                ))}
              </CommandGroup>
            )}
            {backendSupports.newWorktree && (
            <>
            <CommandSeparator />
            <CommandGroup>
              {/* The worktree gets its name from the first prompt, as agent tools name theirs. */}
              <CommandItem value="在 worktree 中工作" data-checked={pending} className={item} onSelect={() => (pick({ kind: 'new-worktree' }), close())}>
                <GitBranchPlusIcon className={icon} />
                在 worktree 中工作
              </CommandItem>
              <CommandItem value="新建 worktree 并命名" className={item} onSelect={() => (close(), setDialog('worktree'))}>
                <PencilIcon className={icon} />
                新建 worktree 并命名…
              </CommandItem>
            </CommandGroup>
            </>
            )}
          </>
        )}
      </SearchTab>
      )}

      <DropdownMenu>
        <DropdownMenuTrigger render={<Button type="button" variant="ghost" size="sm" className={`${tab} shrink-0`} aria-label="选择启动模式" />}>
          <CloudIcon className="size-3.5" />
          云端
          <TabChevron />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start" side="top" className="w-44">
          <DropdownMenuGroup>
            <DropdownMenuLabel className="px-2 py-1.5 text-xs font-normal text-muted-foreground">启动模式</DropdownMenuLabel>
            <DropdownMenuItem className={item} disabled>
              <LaptopIcon className={icon} />
              本地
              <span className="ml-auto text-muted-foreground">需要 desktop</span>
            </DropdownMenuItem>
            <DropdownMenuItem className={item}>
              <CloudIcon className={icon} />
              云端
              <CheckIcon className="ml-auto size-4" />
            </DropdownMenuItem>
          </DropdownMenuGroup>
        </DropdownMenuContent>
      </DropdownMenu>

      <NewProjectDialog open={dialog === 'project'} onOpenChange={(o) => setDialog(o ? 'project' : undefined)} onCreate={(path) => (onCreateProject?.(path), setDialog(undefined))} />
      <NewWorktreeDialog
        open={dialog === 'worktree'}
        onOpenChange={(o) => setDialog(o ? 'worktree' : undefined)}
        projectName={project?.name ?? ''}
        onCreate={(branch) => (pick({ kind: 'isolated', branch }), setDialog(undefined))}
      />
    </div>
  )
}
