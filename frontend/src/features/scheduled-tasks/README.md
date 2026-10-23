# 定时任务页面

此模块负责租户定时任务的管理界面：读取定义及验收开关、展示任务与运行历史，并在 Cloud 部署已开启验收时打开创建表单。它不决定开关值、不计算到期点，也不执行无人值守运行；这些均由 Cloud API 与 worker 负责。

| 文件 | 职责 |
| --- | --- |
| `api.ts` | 包装定时任务、Issue 与智能体的 HTTP 查询和变更。 |
| `scheduled-tasks-page.tsx` | 组合列表、开关控制的创建入口和历史对话框。 |
| `scheduled-task-form.tsx` | 收集并提交定义字段与无人值守授权。 |
| `scheduled-task-history.tsx` | 显示一个任务的 occurrence 历史。 |
| `scheduled-tasks-page.test.tsx` | 验证列表、部署开关、创建入口和关键操作。 |

模块依赖生成 API client、共享 UI 组件及 TanStack Query；租户路由可以依赖本模块的页面组件。保持的约束是：创建入口只相信列表响应的 `acceptanceEnabled`，写入仍由服务端独立校验；测试用 MSW 模拟同一列表响应，不能依赖真实后端。
