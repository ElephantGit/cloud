import { create } from 'zustand'
import { LocalWorkflowBackend } from '@/lib/workflows/data/local-workflow-backend'
import {
  normalizeWorkflowDefinition,
  parseWorkflowGraph,
  type GraphWorkflowRun,
  type WorkflowNodeConversationItem,
  type WorkflowRuntime,
} from '@/lib/workflows/runtime'
import { createMemoryWorkflowRuntime } from '@/lib/workflows/runtime/memory'
import { seedSampleWorkflows } from '@/lib/workflows/samples/seed'

/** A workflow the "运行工作流" menu offers: one with a published version. */
export interface PublishedWorkflow {
  id: string
  name: string
  version: string
}

/** How long each node of a demo run takes; tests shorten it before creating runs. */
export const demoRunClock = { nodeStepMs: 1500 }

interface RunState {
  runs: Record<string, GraphWorkflowRun>
  /** What each run's nodes said, in the order the engine produced it. */
  conversations: Record<string, WorkflowNodeConversationItem[]>
  /** Start values the person entered, kept for the Start node's output. */
  inputs: Record<string, Record<string, unknown>>
}

/** Runs of the in-browser engine, kept current by its change and event streams. */
export const useDemoRuns = create<RunState>()(() => ({ runs: {}, conversations: {}, inputs: {} }))

let runtime: WorkflowRuntime | undefined
const unsubscribes = new Map<string, () => void>()

/**
 * The in-browser stand-in for the workflow run backend: desktop's memory runtime advances the
 * frozen graph on a timer. It lives for the page's lifetime, so runs survive switching pages.
 */
function demoRuntime(): WorkflowRuntime {
  if (runtime) return runtime
  runtime = createMemoryWorkflowRuntime({ nodeStepMs: demoRunClock.nodeStepMs })
  runtime.runs.watch((run) => useDemoRuns.setState((s) => ({ runs: { ...s.runs, [run.id]: run } })))
  return runtime
}

/** Drops every demo run; tests call it between cases. */
export function resetDemoRuns(): void {
  for (const unsubscribe of unsubscribes.values()) unsubscribe()
  unsubscribes.clear()
  runtime?.dispose()
  runtime = undefined
  useDemoRuns.setState({ runs: {}, conversations: {}, inputs: {} })
}

/** The workflow library this browser keeps, with the samples added on a first visit. */
function library(): LocalWorkflowBackend {
  const backend = new LocalWorkflowBackend()
  seedSampleWorkflows(backend)
  return backend
}

/** Workflows that have a published version, newest first, as desktop's menu lists them. */
export function listPublishedWorkflows(): PublishedWorkflow[] {
  return library()
    .list()
    .workflows.flatMap((w) => (w.publishedVersion === null ? [] : [{ id: w.id, name: w.name, version: w.publishedVersion }]))
}

/**
 * Creates a pending run of a workflow's published version in a project, as desktop's run
 * dialog does: the graph is frozen into the run, and Start begins it.
 */
export async function createDemoRun(projectId: string, workflowId: string, name: string): Promise<GraphWorkflowRun> {
  const { workflow, published } = library().get({ workflowId })
  if (!published) throw new Error('这个工作流还没有发布版本')
  const graph = parseWorkflowGraph(published.graph)
  const rt = demoRuntime()
  await rt.host.mount(
    projectId,
    normalizeWorkflowDefinition({
      id: workflow.id,
      name: workflow.name,
      description: graph.description ?? '',
      updatedAt: new Date(Number(published.updatedAt)).toISOString(),
      viewport: graph.viewport,
      globalVariables: graph.globalVariables,
      nodes: graph.nodes,
      edges: graph.edges,
    }),
  )
  const created = await rt.runs.create({ projectId, definitionId: workflow.id })
  const run = name.trim() && name.trim() !== created.name ? await rt.runs.rename(created.id, name) : created
  followConversation(rt, run.id)
  return run
}

/** Keeps the run's node conversation in the store until the run is deleted. */
function followConversation(rt: WorkflowRuntime, runId: string): void {
  const unsubscribe = rt.runs.subscribe(
    runId,
    (event) => {
      if (event.type !== 'node_conversation_item_upserted') return
      useDemoRuns.setState((s) => {
        const list = s.conversations[runId] ?? []
        const index = list.findIndex((item) => item.id === event.item.id)
        const next = index < 0 ? [...list, event.item] : list.map((item, i) => (i === index ? event.item : item))
        return { conversations: { ...s.conversations, [runId]: next } }
      })
    },
    { afterCursor: null },
  )
  unsubscribes.set(runId, unsubscribe)
}

/** Starts a pending run with the Start dialog's prompt and values. */
export async function startDemoRun(runId: string, initialPrompt: string, variables: Record<string, unknown>): Promise<void> {
  const rt = demoRuntime()
  const run = await rt.runs.get(runId)
  const start = run?.definitionSnapshot.nodes.find((node) => node.data.kind === 'start')
  if (start) await rt.runs.updateSnapshotNode(runId, start.id, { input: initialPrompt })
  useDemoRuns.setState((s) => ({ inputs: { ...s.inputs, [runId]: variables } }))
  await rt.runs.start(runId)
}

export async function cancelDemoRun(runId: string): Promise<void> {
  await demoRuntime().runs.cancel(runId)
}

export async function deleteDemoRun(runId: string): Promise<void> {
  await demoRuntime().runs.delete(runId)
  unsubscribes.get(runId)?.()
  unsubscribes.delete(runId)
  useDemoRuns.setState((s) => {
    const { [runId]: _run, ...runs } = s.runs
    const { [runId]: _conversation, ...conversations } = s.conversations
    return { runs, conversations }
  })
}

/** "再跑一次": a fresh pending run of the same frozen graph, so the finished one stays in history. */
export async function rerunDemoRun(runId: string): Promise<GraphWorkflowRun> {
  const rt = demoRuntime()
  const previous = useDemoRuns.getState().runs[runId]
  if (!previous) throw new Error('找不到这次运行')
  const created = await rt.runs.create({ projectId: previous.projectId, definitionId: previous.definitionId })
  const run = await rt.runs.rename(created.id, previous.name)
  followConversation(rt, run.id)
  return run
}
