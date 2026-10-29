# frontend-v2

Cloud v2 的正式 Web 前端，中文界面。技术栈：Vite、React 19、TypeScript、Tailwind CSS 4、shadcn（base-nova）、TanStack Query、zustand、React Router。

## 开发

需要 Node 24。

```sh
npm ci
npm run dev     # http://localhost:5173，/api 代理到 v2-server（:8090）
npm run check   # 类型检查、lint、测试、构建；CI 的 v2 workflow 跑同一条
```

## 结构

```text
src/
  app/          外壳：Provider、路由、布局、左侧图标栏
  features/     每个页面一个目录：chat、multica、workflows、schedules、dashboard
  components/ui shadcn 生成的组件，不手改
  lib/          跨页面共用的工具
```

左侧图标栏的每个图标对应 `src/features/<name>/` 里的一个页面。新增页面：

1. 新建 `src/features/<name>/index.ts`，导出 `Feature`（路径、标题、图标、按需加载的页面）；页面写在同目录的 `page.tsx`。
2. 在 `src/app/features.ts` 里按导航顺序加一行。

各页面互不引用，可以并行开发；页面首次访问时才加载，构建时各自拆包。
