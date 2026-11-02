/** Wire fields used by manual and scheduled container acceptance, without browser-client imports. */
export interface ContainerDetail {
  run: { id: string; status: string; input: string | null; output: string | null; error: string | null; snapshotId: string }
  scopes: { id: string; roundIndex: number; status: string }[]
  nodes: { id: string; nodeId: string; scopeId: string; sessionId: string | null; iteration: number | null; payload: string | null }[]
  variables: { selector: string[]; sourceNodeId: string; value?: unknown }[]
  failedAttempts: unknown[]
  iterationConditionDecisions: Record<string, string>
}

const agent = (prompt: string) => ({ executor: { agentCli: 'official/ora-space.opencode', modelId: 'fixture' }, prompt })
const node = (id: string, kind: string, title: string, x: number, y: number, data: Record<string, unknown> = {}, parentId?: string) => ({ id, type: 'workflow', position: { x, y }, parentId, data: { kind, title, ...data } })
const edge = (source: string, target: string) => ({ id: `${source}-${target}`, source, target })

/** Two isolated feedback rounds, followed by two array iterations in the root scope. */
export const containerWorkflowGraph = JSON.stringify({ schemaVersion: 2, nodes: [
  node('start', 'start', '开始', 0, 0, { inputVariables: [{ name: 'items', valueType: 'array[string]', value: ['第一段', '第二段'] }] }),
  node('loop', 'loop', '反馈循环', 320, 0, { loopConfig: {
    maxIterations: 2, maxIterationsBehavior: 'succeed',
    variables: [{ name: 'draft', valueType: 'string', initial: { kind: 'constant', value: '初稿' }, feedback: ['writer', 'output'] }],
    until: { logic: 'and', conditions: [] }, outputs: [{ name: 'result', variableSelector: ['writer', 'output'] }],
  } }),
  node('entry', 'start', '本轮开始', 40, 80, { containerId: 'loop' }, 'loop'),
  node('writer', 'agent', '改进初稿', 350, 80, { containerId: 'loop', agentConfig: agent('改进：{{#loop.draft#}}') }, 'loop'),
  node('iter', 'iteration', '逐段处理', 1040, 0, { iterationConfig: { iteratorSelector: ['start', 'items'], collectSelector: ['worker', 'output'], errorStrategy: 'continue', maxIterations: 50 } }),
  node('worker', 'agent', '处理段落', 40, 80, { agentConfig: agent('处理第 {{#iter.index#}} 段：{{#iter.item#}}') }, 'iter'),
  node('out', 'output', '输出', 1500, 0, { outputs: [{ name: 'draft', variableSelector: ['loop', 'result'] }, { name: 'paragraphs', variableSelector: ['iter', 'output'] }] }),
], edges: [edge('start', 'loop'), edge('entry', 'writer'), edge('loop', 'iter'), edge('iter', 'worker'), edge('iter', 'out')] })
