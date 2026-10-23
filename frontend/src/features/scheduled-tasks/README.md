# 定时任务（scheduled-tasks）

本模块负责定时任务的**界面**：任务列表与表单、以及单个任务的运行历史。它不拥有任何服务端
事实：任务的接纳、verdict 与产物计数都来自 Cloud 的 REST 资源，本模块只负责请求、呈现和把
用户操作变成 mutation。

## 文件

| 文件 | 职责 |
| --- | --- |
| `api.ts` | 本模块用到的查询与 mutation 包装（列表、历史、创建、启停） |
| `scheduled-tasks-page.tsx` | 页面：列表、创建入口、打开历史 |
| `scheduled-task-form.tsx` | 创建与编辑表单 |
| `scheduled-task-history.tsx` | 运行历史对话框：每次到期点的接纳结果、verdict 与产物数量 |
| `scheduled-tasks-page.test.tsx` | 页面行为测试 |
| `scheduled-task-history.test.tsx` | 历史呈现测试（含「未报告」） |

## 依赖

- 依赖：`@/api`（生成的客户端）、`@/components/ui`、`@/lib` 下的边界适配。
- 允许被谁依赖：路由/页面组合层。

## 不变量

- **「未报告」是一个值，不是缺字段**：运行没有 verdict 时显示「未报告」，不要隐藏这一行，也
  不要把空字符串当作通过。
- **产物计数只表示已登记对象**：`artifactCount` 来自 Cloud 的索引计数，`rejectedArtifactCount`
  是被拒候选；两者不可相加，也不代表用户能否下载（下载由 Cloud 的单独接口按可见性授权）。
- **不在前端缓存服务端事实**：历史与计数只经 TanStack Query 读取，不复制进组件状态。

## 测试

历史测试通过 MSW 提供固定响应，断言渲染结果（结论文案、被拒数量）；不要 mock 内部模块。

创建入口只依据 Cloud 列表响应的 `acceptanceEnabled`；本模块不决定开关，服务端仍独立验证写入。
