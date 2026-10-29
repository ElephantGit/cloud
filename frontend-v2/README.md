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
  lib/          跨页面共用：HTTP 实例（api-client）、登录接口（auth-api）
  test/         单元测试的公共设置和假后端
e2e/            Playwright 端到端测试
```

左侧图标栏的每个图标对应 `src/features/<name>/` 里的一个页面。新增页面：

1. 新建 `src/features/<name>/index.ts`，导出 `Feature`（路径、标题、图标、按需加载的页面）；页面写在同目录的 `page.tsx`。
2. 在 `src/app/features.ts` 里按导航顺序加一行。

各页面互不引用，可以并行开发；页面首次访问时才加载，构建时各自拆包。

## 工作流

工作流页面使用生成的工作流 API hooks，支持选择团队、UUID 游标分页、创建、编辑名称和描述、
粘贴/导出完整 JSON 流程文档、显式保存、发布版本、读取与导出不可变版本、确认归档。
当前是资产管理入口，尚无画布编辑器和工作流执行功能；发布按钮只冻结已保存草稿，
有未保存修改时不能发布或归档。

URL 的 `tenant`、`workflow` 查询参数定位团队和草稿，刷新后重新读取持久状态。
每个编辑器用独立 zustand store 保存本地缓冲；后台查询和版本冲突不会覆盖未保存内容。
冲突时保留编辑内容，可导出备份后检查最新版本并显式重新载入。切页会提示未保存修改，
浏览器刷新/离开使用原生离开提醒；确认放弃才离开。

## 登录

和正式 Cloud 一样，以 `GET /api/v1/me` 判断是否登录：`401` 时在当前位置显示登录页，登录后回到原页面；`403` 表示账号已停用。登录页按 `GET /auth/providers` 为每种登录方式显示一个按钮，`POST /auth/login` 后跳转到返回的地址。v2-server 只提供开发者登录，接入正式 Gateway 时前端不用改。
