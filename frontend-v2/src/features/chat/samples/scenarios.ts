import type * as acp from '@agentclientprotocol/sdk'
import { type Step, script } from './script'

/** One sample conversation in the "示例" project. */
export interface Scenario {
  id: string
  title: string
  /** What the sample is there to show, listed in the empty state and the session header. */
  covers: string
  steps: Step[]
}

const allow: acp.PermissionOption[] = [
  { optionId: 'allow', name: '允许一次', kind: 'allow_once' },
  { optionId: 'always', name: '始终允许', kind: 'allow_always' },
  { optionId: 'reject', name: '拒绝', kind: 'reject_once' },
]

const markdown = script()
  .user('用 Markdown 总结一下 v2 的目录结构')
  .think('用户想要一份结构总结。列出 frontend-v2 和 v2-server 两个目录，再给一个启动命令。')
  .say(`## v2 目录结构

v2 只占用两个目录，**和现有 Cloud 互不影响**：

| 目录 | 作用 | 何时删除 |
|---|---|---|
| \`frontend-v2/\` | 正式的 v2 前端 | 不删除 |
| \`v2-server/\` | 临时单体后端 | 切到正式 Cloud 后 |

前端的每个页面一个目录：

1. \`src/app/\`：外壳、路由和图标栏
2. \`src/features/<name>/\`：各页面
   - 页面自己的组件、store 和测试都放在里面
3. \`src/components/ui/\`：shadcn 生成的组件，不手改

一条命令启动整个系统：

\`\`\`bash
task v2:dev
\`\`\`

前端只经生成的客户端访问后端，例如：

\`\`\`ts
import { useGetMe } from '@/api/me/me'

export function Greeting() {
  const { data } = useGetMe()
  return <p>你好，{data?.displayName}</p>
}
\`\`\`

> 契约里缺的接口先在 v2-server 补上，并登记在它的 README。

详见 [v2-server/README.md](https://github.com/ora-space/cloud/tree/main/v2-server)。`)
  .usage(18_400)
  .end('end_turn', { inputTokens: 12_300, outputTokens: 860, thoughtTokens: 120, totalTokens: 13_280, cachedReadTokens: 9_800 })

const reasoning = script()
  .user('为什么 v2 不直接复用 cloud 现有的 frontend/？')
  .think(
    '先确认现有 frontend/ 的问题在哪。owner 说过它是快速堆出来的，不能用于演示。我需要找相关的决策记录和代码结构来支撑结论。',
  )
  .tool({ id: 'grep-1', title: '搜索 "v2" 相关决策', kind: 'search', input: { pattern: 'frontend-v2', path: 'docs' } })
  .done('grep-1', { output: { matches: 3 } }, 500)
  .tool({ id: 'read-1', title: '读取 AGENTS.md', kind: 'read', locations: [{ path: 'AGENTS.md', line: 12 }] })
  .done('read-1', {}, 400)
  .tool({ id: 'read-2', title: '读取 frontend/AGENTS.md', kind: 'read', locations: [{ path: 'frontend/AGENTS.md' }] })
  .done('read-2', {}, 400)
  .think(
    '两个原因：一是 frontend/ 受 specs 流程和严格的模块文档门禁约束，演示一周内改不动；二是 v2 要能独立构建、独立 CI，和 Cloud 的门禁互不干扰。所以单独建目录更快，以后切到正式 Cloud 时前端也基本不用改。',
  )
  .say(`主要有两个原因：

1. **节奏不同**：\`frontend/\` 走 specs 流程，每个模块都要双语 README 和覆盖率门禁；v2 一周内要能演示，单独目录可以按自己的规则推进。
2. **边界清晰**：v2 只经 \`api/openapi.json\` 生成的客户端访问后端，以后从 v2-server 切到正式 Cloud，前端的数据层不用重写。`)
  .usage(26_000)
  .end()

const tools = script()
  .user('把对话页的标题改成"对话"，然后跑一下检查')
  .say('先找到标题在哪里定义。')
  .tool({ id: 's1', title: '搜索 "对话"', kind: 'search', input: { pattern: '对话', path: 'frontend-v2/src' } })
  .done('s1', { output: { files: ['src/features/chat/index.ts', 'src/features/chat/page.tsx'] } }, 500)
  .tool({ id: 'r1', title: '读取 src/features/chat/index.ts', kind: 'read', locations: [{ path: 'src/features/chat/index.ts' }] })
  .done('r1', {}, 300)
  .tool({ id: 'f1', title: '获取 shadcn Trace 文档', kind: 'fetch', input: { url: 'https://ui.shadcn.com/docs/components/trace' } })
  .done('f1', {}, 600)
  .tool({
    id: 'e1',
    title: '编辑 src/features/chat/index.ts',
    kind: 'edit',
    locations: [{ path: 'src/features/chat/index.ts', line: 6 }],
  })
  .done(
    'e1',
    {
      content: [
        {
          type: 'diff',
          path: 'src/features/chat/index.ts',
          oldText: "export const chat: Feature = {\n  path: 'chat',\n  title: '聊天',\n  icon: MessageSquare,\n}\n",
          newText: "export const chat: Feature = {\n  path: 'chat',\n  title: '对话',\n  icon: MessageSquare,\n}\n",
        },
      ],
    },
    500,
  )
  .tool({ id: 'x1', title: '运行 npm run check', kind: 'execute', input: { command: 'npm run check' } })
  .done(
    'x1',
    {
      output: {
        command: 'npm run check',
        exitCode: 0,
        stdout:
          '> frontend-v2@0.0.0 check\n> npm run typecheck && npm run lint && npm run test && npm run build\n\n✓ typecheck\nFound 0 warnings and 0 errors.\n Test Files  6 passed (6)\n      Tests  48 passed (48)\n✓ built in 2.41s',
      },
    },
    1600,
  )
  .tool({ id: 'x2', title: '运行 npm run e2e', kind: 'execute', input: { command: 'npm run e2e' } })
  .fail('x2', 'Error: PostgreSQL is not reachable at 127.0.0.1:55432\n先运行 task v2:db 启动数据库。', 900, {
    command: 'npm run e2e',
    exitCode: 1,
    stderr: 'Error: PostgreSQL is not reachable at 127.0.0.1:55432',
  })
  .say('标题已经改好，`npm run check` 通过。端到端测试需要先用 `task v2:db` 启动 PostgreSQL，当前环境里没有数据库，所以没跑成。')
  .usage(41_000)
  .end()

const plan = script()
  .user('给对话页加上会话重命名功能')
  .plan([
    ['在 store 里加 rename 动作', 'in_progress'],
    ['左栏会话行支持行内编辑', 'pending'],
    ['补单元测试', 'pending'],
    ['跑 npm run check', 'pending'],
  ])
  .tool({ id: 'p-e1', title: '编辑 src/features/chat/store.ts', kind: 'edit', locations: [{ path: 'src/features/chat/store.ts' }] })
  .done('p-e1', {}, 600)
  .plan([
    ['在 store 里加 rename 动作', 'completed'],
    ['左栏会话行支持行内编辑', 'in_progress'],
    ['补单元测试', 'pending'],
    ['跑 npm run check', 'pending'],
  ])
  .queue('顺便把删除也加上')
  .tool({ id: 'p-e2', title: '编辑 src/features/chat/sidebar.tsx', kind: 'edit', locations: [{ path: 'src/features/chat/sidebar.tsx' }] })
  .done('p-e2', {}, 700)
  .plan([
    ['在 store 里加 rename 动作', 'completed'],
    ['左栏会话行支持行内编辑', 'completed'],
    ['补单元测试', 'in_progress'],
    ['跑 npm run check', 'pending'],
  ])
  .tool({ id: 'p-e3', title: '编辑 src/features/chat/store.test.ts', kind: 'edit', locations: [{ path: 'src/features/chat/store.test.ts' }] })
  .done('p-e3', {}, 600)
  .plan([
    ['在 store 里加 rename 动作', 'completed'],
    ['左栏会话行支持行内编辑', 'completed'],
    ['补单元测试', 'completed'],
    ['跑 npm run check', 'in_progress'],
  ])
  .tool({ id: 'p-x1', title: '运行 npm run check', kind: 'execute', input: { command: 'npm run check' } })
  .done('p-x1', { output: { exitCode: 0, stdout: 'Tests  50 passed (50)\n✓ built in 2.38s' } }, 1200)
  .plan([
    ['在 store 里加 rename 动作', 'completed'],
    ['左栏会话行支持行内编辑', 'completed'],
    ['补单元测试', 'completed'],
    ['跑 npm run check', 'completed'],
  ])
  .say('重命名已完成：双击会话标题即可编辑，回车保存、Esc 取消。')
  .end()
  .user('顺便把删除也加上', {}, 600)
  .say('删除放在会话行的更多菜单里，删除前会确认。')
  .end()

const subagent = script()
  .user('同时调研一下 shadcn 的 chat 组件和 desktop 的对话界面，告诉我该怎么搭')
  .say('我让两个子代理分别去看。')
  .tool({ id: 'task-a', title: '子代理：调研 shadcn chat 组件', kind: 'other', input: { description: '调研 shadcn chat 组件' } })
  .as('child-a', 'root', '调研 shadcn chat 组件', (s) =>
    s
      .think('先列出 shadcn 2026 年新增的 chat 组件，再看 AI Elements 的 pattern。')
      .tool({ id: 'a-f1', title: '获取 ui.shadcn.com/docs/ai-elements', kind: 'fetch' })
      .done('a-f1', {}, 600)
      .tool({ id: 'a-f2', title: '获取 message-scroller 文档', kind: 'fetch' })
      .done('a-f2', {}, 500)
      .say('shadcn 提供 MessageScroller、Message、Bubble、Trace、Code Block，AI Elements 是复制进项目的 pattern，有 Base UI 版本。'),
  )
  .tool({ id: 'task-b', title: '子代理：调研 desktop 对话界面', kind: 'other', input: { description: '调研 desktop 对话界面' } })
  .as('child-b', 'root', '调研 desktop 对话界面', (s) =>
    s
      .tool({ id: 'b-s1', title: '搜索 features/chat', kind: 'search' })
      .done('b-s1', {}, 400)
      .tool({ id: 'b-r1', title: '读取 response-turn.tsx', kind: 'read' })
      .done('b-r1', {}, 300)
      .tool({ id: 'b-r2', title: '读取 activity-phase.tsx', kind: 'read' })
      .done('b-r2', {}, 300)
      .say('desktop 把两段文字之间的工作合成一个活动块，结束后折叠成摘要；子代理按 opencode/child-session 元数据分组。'),
  )
  .done('task-a', { output: { metadata: { sessionID: 'child-a', status: 'completed' } } }, 200)
  .done('task-b', { output: { metadata: { sessionID: 'child-b', status: 'completed' } } }, 200)
  .say(`两边的结论合起来：

- 组件用 shadcn 的 **MessageScroller + Message + Trace**，工具和思考都画成 Trace 行。
- 结构沿用 desktop：两段文字之间的工作合成一个活动块，子代理按元数据分组嵌套。`)
  .usage(63_000)
  .end()

const permission = script()
  .user('把 node_modules 删掉重新装一遍依赖')
  .say('需要执行删除命令，先请你确认。')
  .tool({ id: 'rm', title: '运行 rm -rf node_modules && npm ci', kind: 'execute', input: { command: 'rm -rf node_modules && npm ci' } })
  .ask(
    'perm-1',
    { toolCallId: 'rm', title: '运行 rm -rf node_modules && npm ci', kind: 'execute', rawInput: { command: 'rm -rf node_modules && npm ci' } },
    allow,
    {
      allow: (s) => okInstall(s),
      always: (s) => okInstall(s),
      reject: (s) =>
        s
          .fail('rm', '用户拒绝了这次执行。', 200)
          .say('好的，没有删除。需要的话可以只运行 `npm ci`，它会自动清理旧依赖。')
          .end(),
    },
  )

function okInstall(s: ReturnType<typeof script>) {
  s.done('rm', { output: { exitCode: 0, stdout: 'added 412 packages in 9s' } }, 1400)
    .say('依赖已经重新安装，共 412 个包。')
    .end()
}

const changes = script()
  .user('给对话页加暗色模式')
  .say('我在这个对话的沙盒里改，项目仓库不受影响。')
  .tool({ id: 'w-e1', title: '编辑 src/index.css', kind: 'edit', locations: [{ path: 'src/index.css' }] })
  .done(
    'w-e1',
    {
      content: [
        {
          type: 'diff',
          path: 'src/index.css',
          oldText: ':root {\n  --background: oklch(1 0 0);\n}\n',
          newText: ':root {\n  --background: oklch(1 0 0);\n}\n\n.dark {\n  --background: oklch(0.145 0 0);\n}\n',
        },
      ],
    },
    500,
  )
  .tool({ id: 'w-e2', title: '新建 src/components/theme-toggle.tsx', kind: 'edit' })
  .done(
    'w-e2',
    {
      content: [
        {
          type: 'diff',
          path: 'src/components/theme-toggle.tsx',
          oldText: null,
          newText:
            "export function ThemeToggle() {\n  const toggle = () => document.documentElement.classList.toggle('dark')\n  return <button onClick={toggle}>切换主题</button>\n}\n",
        },
      ],
    },
    600,
  )
  .tool({ id: 'w-e3', title: '编辑 src/app/nav-rail.tsx', kind: 'edit' })
  .done(
    'w-e3',
    {
      content: [
        {
          type: 'diff',
          path: 'src/app/nav-rail.tsx',
          oldText: '      <UserMenu />\n',
          newText: '      <ThemeToggle />\n      <UserMenu />\n',
        },
      ],
    },
    500,
  )
  .say('暗色模式已加上：图标栏底部有切换按钮。改动都在这个对话自己的沙盒里，不会写回项目仓库，也不影响其他对话。')
  .end()

const failures = script()
  .user('跑一下单元测试，再跑一下数据迁移脚本')
  .tool({ id: 't1', title: '运行 npm test', kind: 'execute', input: { command: 'npm test' } })
  .fail('t1', '1 个测试失败', 1500, {
    command: 'npm test',
    exitCode: 1,
    suites: [
      {
        name: 'src/features/chat/model/conversation.test.ts',
        tests: [
          { name: '合并连续的文本片段', status: 'passed', durationMs: 3 },
          { name: '计时思考过程', status: 'passed', durationMs: 2 },
          {
            name: '取消时把未结束的工具标为已取消',
            status: 'failed',
            durationMs: 4,
            error: "AssertionError: expected 'in_progress' to be 'cancelled'\n  at conversation.test.ts:118:42",
          },
          { name: '记录权限确认的结果', status: 'skipped' },
        ],
      },
      {
        name: 'src/features/chat/model/display.test.ts',
        tests: [
          { name: '把两段文字之间的工作合成一块', status: 'passed', durationMs: 2 },
          { name: '子代理嵌套', status: 'passed', durationMs: 3 },
        ],
      },
    ],
  })
  .tool({ id: 't2', title: '运行 node scripts/migrate.js', kind: 'execute', input: { command: 'node scripts/migrate.js' } })
  .fail(
    't2',
    `TypeError: Cannot read properties of undefined (reading 'rows')
    at runMigration (/app/scripts/migrate.js:42:18)
    at main (/app/scripts/migrate.js:88:5)
    at Module._compile (node:internal/modules/cjs/loader:1554:14)
    at Object..js (node:internal/modules/cjs/loader:1706:10)
    at Module.load (node:internal/modules/cjs/loader:1289:32)`,
    800,
  )
  .say('有一个测试失败：取消时没有把进行中的工具标成"已取消"。迁移脚本在第 42 行读到了空的查询结果，需要先检查数据库连接。')
  .end()

const sources = script()
  .user('ACP 协议里工具调用有哪些状态？给出处')
  .tool({ id: 'src-f1', title: '获取 agentclientprotocol.com/protocol/tool-calls', kind: 'fetch' })
  .done('src-f1', {}, 700)
  .tool({ id: 'src-f2', title: '获取 ACP schema', kind: 'fetch' })
  .done('src-f2', {}, 500)
  .say(
    'ACP 的工具调用有四种状态：`pending`、`in_progress`、`completed` 和 `failed` [1]。状态随 `tool_call_update` 更新，每次只需发送变化的字段 [2]。取消不是一种状态：被取消的轮次里未结束的工具没有结果，客户端需要自己处理 [1]。',
  )
  .block({ type: 'resource_link', name: 'Tool Calls', title: 'Tool Calls - Agent Client Protocol', uri: 'https://agentclientprotocol.com/protocol/tool-calls', description: '工具调用的生命周期与状态' })
  .block({ type: 'resource_link', name: 'Schema', title: 'Schema - Agent Client Protocol', uri: 'https://agentclientprotocol.com/protocol/schema', description: 'ToolCallUpdate 的字段定义' })
  .end()

const context = script()
  .user('先帮我读一下整个 v2-server 的代码')
  .tool({ id: 'c-r1', title: '读取 v2-server/main.go', kind: 'read' })
  .done('c-r1', {}, 300)
  .tool({ id: 'c-r2', title: '读取 v2-server/internal/server/server.go', kind: 'read' })
  .done('c-r2', {}, 300)
  .usage(88_000)
  .say('读完了：入口在 `main.go`，路由和登录在 `internal/server`，存储用 pgx 直连 PostgreSQL。')
  .end('end_turn', { inputTokens: 86_200, outputTokens: 540, totalTokens: 86_740, cachedReadTokens: 40_100 })
  .checkpoint('读完 v2-server 之后')
  .model('Claude Opus 5.5')
  .user('换个更强的模型，帮我设计会话接口')
  .think('会话接口要覆盖列表、新建、发消息、取消和事件流，事件流沿用 ACP 的 session/update。')
  .say(`建议的接口：

- \`GET /api/v1/tenants/{tid}/chat-sessions\`
- \`POST /api/v1/tenants/{tid}/chat-sessions\`
- \`POST …/chat-sessions/{sid}/prompts\`
- \`GET …/chat-sessions/{sid}/events\`（SSE）`)
  .usage(131_000)
  .end('end_turn', { inputTokens: 128_400, outputTokens: 1_320, thoughtTokens: 610, totalTokens: 130_330 })

const interrupted = script()
  .user('把整个仓库的注释都翻译成中文')
  .say('好的，我先列出所有源文件，然后逐个')
  .tool({ id: 'i-s1', title: '搜索所有 .go 文件', kind: 'search' })
  .end('cancelled', undefined, 900)
  .user('算了，换个问题：v2-server 用的是哪个端口？')
  .say('默认监听 `127.0.0.1:8090`，可以用 `V2_ADDR` 改')
  .error('模型连接中断：上游返回 529 Overloaded，请稍后重试。', 500)

const attachments = script()
  .user(
    '照着这张截图调一下左栏的间距，用上 UI 规范插件',
    {
      attachments: [
        { id: 'a1', name: '左栏截图.png', mimeType: 'image/png', size: 184_320, url: sidebarSketch() },
        { id: 'a2', name: 'design-tokens.json', mimeType: 'application/json', size: 2_140 },
      ],
      plugins: [{ id: 'ui-guidelines', name: 'UI 规范' }],
    },
  )
  .think('截图里会话行的上下间距大约 6px，项目标题和第一行之间多了 4px。按 UI 规范，行高统一 32px。')
  .say('按截图和 UI 规范：会话行高统一为 32px，项目标题和第一行之间去掉多余的 4px。')
  .end()

/** A tiny inline picture so the attachment sample shows a real image preview. */
function sidebarSketch() {
  const svg =
    "<svg xmlns='http://www.w3.org/2000/svg' width='240' height='160'><rect width='240' height='160' fill='#f4f4f5'/><rect x='12' y='14' width='90' height='10' rx='3' fill='#a1a1aa'/><rect x='24' y='36' width='150' height='8' rx='3' fill='#d4d4d8'/><rect x='24' y='54' width='120' height='8' rx='3' fill='#d4d4d8'/><rect x='24' y='72' width='160' height='8' rx='3' fill='#d4d4d8'/><rect x='12' y='98' width='70' height='10' rx='3' fill='#a1a1aa'/><rect x='24' y='120' width='140' height='8' rx='3' fill='#d4d4d8'/></svg>"
  return `data:image/svg+xml;utf8,${encodeURIComponent(svg)}`
}

/** The samples, in sidebar order. */
export const scenarios: Scenario[] = [
  { id: 'markdown', title: '回答格式：Markdown 与代码块', covers: 'Message、Code Block', steps: markdown.steps },
  { id: 'reasoning', title: '思考过程', covers: 'Reasoning、Chain of Thought', steps: reasoning.steps },
  { id: 'tools', title: '工具调用：读、搜、改、执行', covers: 'Tool、Trace、Terminal、Code Block（diff）、Task', steps: tools.steps },
  { id: 'plan', title: '计划与排队', covers: 'Plan、Queue', steps: plan.steps },
  { id: 'subagent', title: '子代理', covers: 'Subagent', steps: subagent.steps },
  { id: 'permission', title: '权限确认', covers: 'Confirmation', steps: permission.steps },
  { id: 'failures', title: '测试结果与报错', covers: 'Test Results、Stack Trace', steps: failures.steps },
  { id: 'sources', title: '引用与来源', covers: 'Inline Citation、Sources', steps: sources.steps },
  { id: 'context', title: '上下文、模型切换与检查点', covers: 'Context、Model Selector、Checkpoint', steps: context.steps },
  { id: 'interrupted', title: '中断与失败', covers: '取消、出错', steps: interrupted.steps },
  { id: 'attachments', title: '附件与插件', covers: 'Prompt Input、Attachment', steps: attachments.steps },
  { id: 'changes', title: '改代码与改动文件', covers: 'File Tree', steps: changes.steps },
]
