# frontend-v2

Cloud v2 的正式 Web 前端，中文界面。技术栈：Vite、React 19、TypeScript、Tailwind CSS 4、shadcn（base-nova）、TanStack Query、zustand、React Router，客户端由 orval 从 `api/openapi.json` 生成。

## 开发

需要 Node 24、Go、Docker（跑 PostgreSQL）和 [Task](https://taskfile.dev)。在仓库根目录：

```sh
task v2:dev    # PostgreSQL + v2-server（:8090）+ 前端（http://localhost:5173）
task v2:e2e    # Playwright 端到端测试
```

打开页面后点"开发者登录"，以演示用户进入。只改前端时也可以在本目录：

```sh
npm ci
npm run dev           # /api、/auth、/healthz 代理到 v2-server（V2_SERVER，默认 :8090）
npm run check         # 类型检查、lint、单元测试、构建；CI 的 v2 workflow 跑同一条
npm run api:generate  # api/openapi.json 变化后重新生成 src/api
npm run e2e           # 端到端测试，自己拉起 v2-server（:8091）和前端（:5174）
```

端到端测试默认连接 `task v2:db` 的 PostgreSQL（库 `ora_v2_e2e`，可用 `E2E_DATABASE_URL` 指定）。第一次运行前执行 `npx playwright install chromium`；本机已有 Chromium 时，也可以用 `PW_CHROMIUM_PATH` 指定可执行文件。

## 结构

```text
src/
  app/          外壳：Provider、路由、布局、左侧图标栏、登录（auth/）
  features/     每个页面一个目录：chat、multica、workflows、schedules、dashboard
  api/          orval 生成的客户端，不手改
  components/ui shadcn 生成的组件，不手改
  lib/          跨页面共用：HTTP 实例（api-client）、登录接口（auth-api）、代码高亮（highlighter，shadcn 生成）
  hooks/        shadcn 生成的 hook
  test/         单元测试的公共设置和假后端
e2e/            Playwright 端到端测试
```

左侧图标栏的每个图标对应 `src/features/<name>/` 里的一个页面。新增页面：

1. 新建 `src/features/<name>/index.ts`，导出 `Feature`（路径、标题、图标、按需加载的页面）；页面写在同目录的 `page.tsx`。
2. 在 `src/app/features.ts` 里按导航顺序加一行。

各页面互不引用，可以并行开发；页面首次访问时才加载，构建时各自拆包。

## 对话

`src/features/chat/` 是对话页，界面对齐 desktop 的对话界面。组件用 shadcn 的 chat 组件（MessageScroller、Message、Bubble、Trace、Code Block、Attachment、Marker），复制进来的 AI Elements pattern 放在 `components/`。Code Block 的高亮依赖 shadcn 一起生成的 `src/lib/highlighter.tsx` 和 `src/hooks/use-highlighted-code.ts`。

- `model/`：把 ACP 事件（`session/update`、权限请求、轮次结束）归约成界面条目，再排成显示用的行（活动块、子代理分组、改动文件）。真实会话和样例走同一套代码。
- `samples/`：录好的示例对话，覆盖工具调用、思考、计划、子代理、权限确认、测试结果、报错、引用、上下文、检查点、附件和插件，在左栏"示例"项目里回放。
- `store.ts`：会话、项目和回放状态。左栏按项目分组，项目下先列主目录里的会话，再按分支列出各 worktree 和它的会话；拖动项目可调整顺序，拖动左栏右边缘可调宽度或收起。项目顺序和左栏布局保存在浏览器本地。

新对话的排版照 desktop：没有会话选中时显示新对话，标题和输入框在页面中间，输入框上方选项目、主目录或 worktree、启动模式（网页上只有云端）；第一条消息发出时才建会话，输入框滑到底部。左栏顶部的"新建对话"和项目、worktree 行上的 `+` 都打开新对话。项目和分支菜单可以搜索；新建项目填主目录路径，项目名取最后一级目录；选"在 worktree 中工作"时先不取名，第一条消息发出后按消息内容命名（取开头几个英文词，没有英文词时用 `ora/<形容词>-<名词>`，见 `model/branch-name.ts`；接入 Agent 后改由 Agent 命名），也可以"新建 worktree 并命名"自己起名。输入框右下角先选 Agent 再选它的模型，模型可以搜索；`store.ts` 里的 Agent 和模型清单是演示数据，等 v2-server 报告可用的 Agent 后改用接口。

v2-server 还不能运行 Agent，也没有会话接口：现在非示例会话发消息得到一条注明"演示回复"的回答。接入后，会话列表、发消息和事件流改走 v2-server，项目顺序也随之存到后端。

## 工作流

`src/features/workflows/` 是工作流页，从 desktop（`ora-space/desktop` main `cab59a3c`）的工作流编辑器原样移植：左栏是工作流列表（新建、复制、重命名、删除、导入、导出），右边是 React Flow 画布、节点检查器、撤销重做、草稿自动保存、发布和版本历史。图结构就是 desktop 的工作流文档，导出的文件和 desktop 互通。

| 目录 | 来自 desktop |
|---|---|
| `editor/` | `packages/app-shell/src/features/workflow-editor` |
| `node-chrome/` | `packages/app-shell/src/features/workflow-node-chrome` |
| `mock/`、`runtime/` | `packages/workflow-mock`、`packages/workflow-runtime` |
| `composer-core/`、`prompt-composer/` | `packages/editor` 的 composer 和 app-shell 的提示词编辑器（tiptap） |
| `data/workflows.ts`、`data/workflow-analysis.ts` | `packages/app-shell/src/state/data` |
| `contracts/dto/` | `packages/contracts` 生成的 DTO 类型 |

复制来的文件只改了 import 路径，以及几处 v2 的 lint 或 TypeScript 设置不接受的写法。desktop 依赖的宿主能力在 `host/` 里换成浏览器实现：`ui.ts` 转出 v2 生成的 shadcn 组件；`i18n.ts` 只加载 desktop 的 zh-CN 文案；`platform.tsx` 用浏览器下载代替保存对话框；`window-chrome.tsx` 去掉了 desktop 的窗口拖拽和窗口按钮。页面自带一个 QueryClient，沿用 desktop 的缓存策略（`staleTime` 30 秒），编辑器的缓存补丁和自动保存依赖这一点。

左栏和对话页的聊天记录一样：拖动右边缘调宽度，拖过最小宽度就收起，也可以聚焦边缘后用方向键调整、按回车切换；收起后从编辑器标题栏的按钮展开。宽度和是否收起保存在浏览器本地（`host/ui-store.ts`）。提示条右上角有关闭按钮。

`samples/` 是样例工作流：`authored.ts` 写了两份，代码审查（开始 → Agent → 条件分支 → 两个 Agent → 变量聚合器 → 输出）和逐段翻译（迭代）；另外三份是 desktop 自带的 Loop 样例（`artifacts/workflow-loop-examples/`），原样复制，导入时把 Agent 改成演示执行器、去掉角色。浏览器第一次打开工作流页时，`samples/seed.ts` 按导入对话框的同一条路径把它们建好，并发布为 `v1`。五份合起来覆盖全部八种可执行节点。

v2-server 还没有工作流接口，现在由 `data/local-workflow-backend.ts` 在浏览器 localStorage 里代替 desktop 的后端，规则照搬 desktop 的 Rust 实现：名称去空格后不能为空、不能重名；一个可编辑的 `draft` 加不可变的发布版本；版本名不能叫 `draft`，也不能含 `/`、`\`、控制字符；不能删除草稿和当前生效的版本；发布时检查图能否从开始节点投影成可执行图（多余节点不阻塞发布）；analyze 标出未参与运行的节点和不认识的节点类型。desktop 引擎对每个节点配置的检查没有搬过来。Agent、模型、角色、Skill 和 MCP 清单是 `data/demo-catalog.ts` 里的演示数据。页面不运行工作流。接入 v2-server 后，只需把 `data/contracts-client.ts` 换成调生成的客户端。

## 登录

和正式 Cloud 一样，以 `GET /api/v1/me` 判断是否登录：`401` 时在当前位置显示登录页，登录后回到原页面；`403` 表示账号已停用。登录页按 `GET /auth/providers` 为每种登录方式显示一个按钮，`POST /auth/login` 后跳转到返回的地址。v2-server 只提供开发者登录，接入正式 Gateway 时前端不用改。
