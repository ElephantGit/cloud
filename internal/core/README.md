# internal/core：权威领域与持久化

[中文](README.md) | [English](README.en.md)

本包拥有业务状态、授权、事务与恢复判定。HTTP 和 gRPC 适配层将请求翻译到 Store；跨模块契约见 [文档地图](../../docs/README.md)。

## 代码地图

| 文件组 | 包内职责 | 跨模块说明 |
|---|---|---|
| `store.go`、`public.go`、`control.go` | 注入 SQL pool、事务包装、请求分发、幂等与 Fault | [持久化与事务](../../docs/core/persistence.md) |
| `auth.go`、`tenant.go`、`join.go`、`space*.go` | 已验证身份解析、租户与成员作用域 | [身份与租户](../../docs/identity/tenancy.md) |
| `node*.go`、`runtime_*.go`、`run_workspace.go` | Node 事实、控制会话、围栏与运行 Workspace | [运行时控制](../../docs/runtime/runtime-control.md) |
| `repository_*.go`、`git_*.go` | Binding、同步/发布 Operation、GitGrant 与凭据元数据 | [外部 Git 线](../../docs/repositories/external-git.md) |
| `issue*.go`、`collaboration*.go`、`form_descriptor.go` | Issue 聚合、时间线、交互和消费方协作端口 | [Issue 协作](../../docs/collaboration/interactions.md) |
| `agent_run_*.go`、`execution*.go`、`thread*.go`、`business_hooks.go` | 运行生命周期、执行登记、Thread 与同事务结算钩子 | [IssueRun](../../docs/agent-runs/issue-runs.md)、[Controller](../../docs/runtime/controller-integration.md) |
| `task_conversation_*.go`、`thread_preview.go` | 任务级有序消息、同任务 Agent 串行交接与暂停屏障、从 Node 可见正文冻结的后续提示、配置缺口提示和有界进程内实时预览；Agent 初始提示只读冻结的运行输入 | [Thread](../../docs/agent-runs/threads.md)、[IssueRun](../../docs/agent-runs/issue-runs.md) |
| `revision*.go`、`session_*.go` | Revision 登记、归档读取、会话列表与统计 | [Revision](../../docs/agent-runs/revisions.md)、[会话读取](../../docs/agent-runs/session-reads.md) |
| `model_*.go` | 个人配置、冻结绑定、授权摘要与请求观察；不解密 API Key | [模型访问](../../docs/models/model-access.md) |
| `plugins.go`、`plugin_*.go`、`workflow_asset_imports.go` | 含 Agent 声明副本的市场快照、空间选择、冻结声明关联的 Node 安装计划和资产导入账本 | [插件市场](../../docs/plugins/marketplace.md) |
| `workflow*.go` | 图、发布快照、准入、运行身份、调度器；Issue Confirm/manual 与直接启动共用的 WorkflowRun 创建、`workflow_invocation_ref` 关联，以及调度器把 WorkflowRun 状态投影回关联 IssueRun | [工作流](../../docs/workflows/workflows.md)、[Issue 协作](../../docs/collaboration/interactions.md) |
| `hub.go`、`signals.go` | 提交后失效通知和控制信号；不承载权威状态 | [HTTP API](../../docs/core/http-api.md) |

## Store 与事务边界

- `NewStore` 从注入的 GORM 获取 `*sql.DB`，装配真实 Workflow Directory/Forms；不连接、迁移或建立全局数据库。`NewDevelopmentStore` 额外允许旧 clone 夹具。
- `transact` 在 PostgreSQL 事务内取得 `pg_advisory_xact_lock(67420911)`，处理领域 Fault 和提交后的通知。调用方必须使用传入的事务执行钩子，不能嵌套开启 Store 事务。
- 外部网络、Git、文件和对象存储调用必须在事务外；返回后按原身份、版本和授权重新核对。归档读取、发布源核对与凭据确认有专用入口。
- `Object` 是边界 JSON 值；`Fault` 与 `ErrorCode` 将可预期错误和数据库失败转换为安全公开错误。领域内部的 panic 只在事务控制边界转换，不能吞掉未知 panic。
- `Store` 的端口与策略时长由命令装配。开发协作端口见 [collab](../collab/README.md)；物理执行证据不由模拟结果代替。

迁移目录与完整文件名见 [migrations](migrations/README.md)。包内 `*_db_test.go` 需独立真实 PostgreSQL schema；跨 HTTP、Git 与存储边界的覆盖在 [integration](../../integration/README.md)。
