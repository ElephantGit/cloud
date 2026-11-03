import { useState, type ReactNode } from 'react'
import { DndContext, DragOverlay, PointerSensor, pointerWithin, useDraggable, useDroppable, useSensor, useSensors } from '@dnd-kit/core'
import { AtSign, Bot, CornerDownRight, ListTree, UserRound, Users } from 'lucide-react'
import type { PendingMention, Task } from './tasks-api/generated'
import { assigneeLabel, projectLabel, taskStatusLabel } from './task-model'
import { BOARD_COLUMNS, boardDraggable, boardMove, isAgentRunning } from './task-views'

type Projects = { id: string; name: string }[]
type Mentions = ReadonlyMap<string, PendingMention>
type Families = ReadonlyMap<string, string>

const ASSIGNEE_ICONS = { human: UserRound, agent: Bot, squad: Users }

/** Who owns the task, with an icon for a person, an Agent or a squad; 未指派 otherwise. */
function TaskAssignee({ task }: { task: Task }) {
  const Icon = task.assignee ? ASSIGNEE_ICONS[task.assignee.type] : null
  return <span className={task.assignee ? 'mc-assignee-tag' : 'mc-assignee-tag none'} title="负责">
    {Icon && <Icon size={12} aria-hidden="true" />}{assigneeLabel(task)}
  </span>
}

/** What is happening on the task: the running Agent and who started it, or a run waiting on its starter. */
function TaskWorker({ task }: { task: Task }) {
  const run = task.activeRun
  if (isAgentRunning(task)) {
    return <span className="mc-running"><i className="mc-pulse" aria-hidden="true" />{run?.startedByName} 发起 · 执行中</span>
  }
  if (run?.state === 'idle') return <span className="mc-task-hint"><Bot size={12} />{run.startedByName} 发起 · 等待追问</span>
  return task.assignee?.type === 'squad' ? <span className="mc-task-hint">小队负责，不自动分派</span> : null
}

/** The requirement a subtask was split from, or how many subtasks a requirement has. */
function FamilyTag({ task, label }: { task: Task; label?: string }) {
  if (!label) return null
  const Icon = task.parentId ? CornerDownRight : ListTree
  return <span className="mc-family-tag"><Icon size={11} aria-hidden="true" />{label}</span>
}

/** Someone @-mentioned the viewer here and is waiting on their reply. */
function MentionTag({ mention }: { mention?: PendingMention }) {
  if (!mention) return null
  return <span className="mc-mention-tag" title="回复后提醒消失"><AtSign size={11} aria-hidden="true" />{mention.authorName} @ 了你</span>
}

/** Tasks as rows. */
export function TaskRows({ tasks, projects, mentions, families = new Map(), onOpen }: { tasks: Task[]; projects: Projects; mentions: Mentions; families?: Families; onOpen: (task: Task) => void }) {
  return <ul className="mc-tasks">{tasks.map(item => <li key={item.id}>
    <button type="button" className="mc-task-row" aria-label={`查看任务 ${item.title}`} onClick={() => onOpen(item)}>
      <span className={`mc-status ${item.status}`}>{taskStatusLabel(item.status)}</span>
      <span className="mc-task-main">
        <strong>{item.title}</strong>
        <small>{projectLabel(projects, item.projectId)} · {item.createdByName} 创建</small>
        <FamilyTag task={item} label={families.get(item.id)} />
      </span>
      <MentionTag mention={mentions.get(item.id)} />
      <TaskWorker task={item} />
      <TaskAssignee task={item} />
    </button>
  </li>)}</ul>
}

/** A board card's face, shared by the card in its column and the one lifted while dragging. */
function CardFace({ task, projects, mention, family }: { task: Task; projects: Projects; mention?: PendingMention; family?: string }) {
  return <>
    <MentionTag mention={mention} />
    <strong>{task.title}</strong>
    <FamilyTag task={task} label={family} />
    <small>{projectLabel(projects, task.projectId)} · {task.createdByName} 创建</small>
    <TaskAssignee task={task} />
    <TaskWorker task={task} />
  </>
}

/**
 * One card. A short pointer drag picks it up (a click still opens the task); a running Agent's card
 * and finished work cannot be picked up, and say why on hover.
 */
function BoardCard({ task, projects, mention, family, movable, onOpen }: { task: Task; projects: Projects; mention?: PendingMention; family?: string; movable: boolean; onOpen: (task: Task) => void }) {
  const pickable = movable ? boardDraggable(task) : { ok: false, reason: '' }
  const { setNodeRef, listeners, isDragging } = useDraggable({ id: task.id, disabled: !pickable.ok })
  return <li ref={setNodeRef} className={isDragging ? 'mc-card-lifted' : undefined} {...listeners}>
    <button type="button" className="mc-card-task" aria-label={`查看任务 ${task.title}`} title={movable ? pickable.reason || '拖到其他列改状态' : undefined} onClick={() => onOpen(task)}>
      <CardFace task={task} projects={projects} mention={mention} family={family} />
    </button>
  </li>
}

/**
 * One status column. While a card is lifted it shows whether the card may land here: open to a
 * legal move, dimmed otherwise. A dimmed column still takes the drop, so letting go over it says
 * why instead of landing the card in a neighbor.
 */
function BoardColumn({ status, children, count, lifted }: { status: string; children: ReactNode; count: number; lifted?: Task }) {
  const move = lifted ? boardMove(lifted, status) : undefined
  const { setNodeRef, isOver } = useDroppable({ id: status })
  const state = !move || lifted?.status === status ? '' : move.ok ? (isOver ? ' mc-drop-over' : ' mc-drop-open') : ' mc-drop-closed'
  return <section ref={setNodeRef} className={`mc-column${state}`} role="listitem" aria-label={`${taskStatusLabel(status)}列`}>
    <header><span className={`mc-status ${status}`}>{taskStatusLabel(status)}</span><b>{count}</b></header>
    <ul>{children}</ul>
  </section>
}

/**
 * Tasks as a board, one column per status. Dragging a card to another column makes the same move
 * as the task's own buttons, so only the next legal step is open: 待验收 is never skipped, a running
 * Agent's card stays put, and an Agent task enters 进行中 only by starting its run. A refused drop
 * says why through `onRefuse`; the keyboard keeps the task page's buttons.
 */
export function TaskBoardColumns({ tasks, projects, mentions, families = new Map(), showCancelled, onOpen, onMove, onRefuse }: {
  tasks: Task[]; projects: Projects; mentions: Mentions; families?: Families; showCancelled: boolean; onOpen: (task: Task) => void
  onMove?: (task: Task, status: string) => void; onRefuse?: (reason: string) => void
}) {
  const columns: string[] = showCancelled ? [...BOARD_COLUMNS, 'cancelled'] : [...BOARD_COLUMNS]
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 6 } }))
  const [liftedId, setLiftedId] = useState<string>()
  const lifted = onMove ? tasks.find(t => t.id === liftedId) : undefined
  const drop = (to?: string) => {
    setLiftedId(undefined)
    if (!lifted || !to || to === lifted.status) return
    const move = boardMove(lifted, to)
    if (move.ok) onMove?.(lifted, to)
    else if (move.reason) onRefuse?.(move.reason)
  }
  const titleOf = (id: string | number) => tasks.find(t => t.id === id)?.title ?? '任务'
  const columnOf = (id?: string | number) => (id === undefined ? '' : taskStatusLabel(String(id)))
  const announcements = {
    onDragStart: ({ active }: { active: { id: string | number } }) => `拿起了「${titleOf(active.id)}」。`,
    onDragOver: ({ active, over }: { active: { id: string | number }; over: { id: string | number } | null }) =>
      over ? `「${titleOf(active.id)}」在「${columnOf(over.id)}」列上方。` : `「${titleOf(active.id)}」不在任何列上方。`,
    onDragEnd: ({ active, over }: { active: { id: string | number }; over: { id: string | number } | null }) =>
      over ? `「${titleOf(active.id)}」放到了「${columnOf(over.id)}」列。` : `「${titleOf(active.id)}」放回原处。`,
    onDragCancel: ({ active }: { active: { id: string | number } }) => `取消拖动，「${titleOf(active.id)}」放回原处。`,
  }
  // The column under the pointer is the target, never the one the lifted card overlaps most.
  return <DndContext sensors={sensors} collisionDetection={pointerWithin} accessibility={{ announcements, screenReaderInstructions: { draggable: '用指针拖到其他列可以改状态；键盘请在任务页用推进按钮。' } }} onDragStart={({ active }) => setLiftedId(String(active.id))}
    onDragCancel={() => setLiftedId(undefined)} onDragEnd={({ over }) => drop(over ? String(over.id) : undefined)}>
    <div className="mc-board" role="list" aria-label="任务看板">
      {columns.map(status => {
        const items = tasks.filter(t => t.status === status)
        return <BoardColumn key={status} status={status} count={items.length} lifted={lifted}>
          {items.map(item => <BoardCard key={item.id} task={item} projects={projects} mention={mentions.get(item.id)} family={families.get(item.id)} movable={!!onMove} onOpen={onOpen} />)}
          {items.length === 0 && <li className="mc-column-empty">暂无</li>}
        </BoardColumn>
      })}
    </div>
    <DragOverlay dropAnimation={null}>
      {lifted && <div className="mc-card-task mc-card-overlay"><CardFace task={lifted} projects={projects} mention={mentions.get(lifted.id)} family={families.get(lifted.id)} /></div>}
    </DragOverlay>
  </DndContext>
}
