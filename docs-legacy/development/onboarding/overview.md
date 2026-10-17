# Ora Cloud — 项目概览（新员工向）

欢迎！这份文档帮你在 15 分钟内建立对 Ora Cloud 的整体认识。想上手改代码看 [../agent/](../agent/)（那份是写给 AI 助手和
已经熟悉项目的人的）。

## 这是什么

Ora Cloud 是一个**云端开发环境编排平台**的权威存储与控制面。它管理：组织（tenant）、成员、
项目（project，一个 Git 仓库对应的开发环境）、工作区（workspace，仓库的一个 checkout +
运行沙箱），以及把这些从「想要的状态」推进到「实际状态」的**持久化异步操作**（operation）。

在此之上，我们从参考项目 **Multica** 迁移了一个 **Issue 看板（Kanban）**，作为第一个
「纯业务」资源。

> 技术栈：Go 1.27 + Gin + GORM（仅连接池，业务逻辑用手写 SQL）+ PostgreSQL 17。

## 一张图看懂请求怎么流转

```
浏览器 / CLI / Gateway
        │  HTTPS（两个 JWT：service + user）
        ▼
┌─────────────────────────────────────────┐
│  internal/api/router                    │  路由白名单 + 双 JWT 校验 + 严格 JSON
│  （不认识的路由/字段直接 400/401）        │
└─────────────────────────────────────────┘
        │  core.PublicRequest
        ▼
┌─────────────────────────────────────────┐
│  core.Store.Public                      │  每条请求 = 一个短事务
│   pg_advisory_xact_lock ── 全局写锁       │  identity → membership → dispatch
└─────────────────────────────────────────┘
        │  手写 SQL（*sql.Tx）
        ▼
   PostgreSQL 17（权威存储）
```

**几个要点：**

- 所有写操作都在**同一个全局 advisory 锁**里串行执行 —— 简单、正确，但是一个明确的吞吐上限。
- **幂等**：POST/DELETE 必须带 `Idempotency-Key`，重复提交返回第一次的结果。
- **乐观并发**：更新/删除带 `version`，过期就 409，防止覆盖别人的改动。
- **软删除**：业务行只标记 `deleted_at`，不真正删行。

## 主要模块

| 模块 | 目录 | 干嘛的 |
| --- | --- | --- |
| HTTP 层 | `internal/api/router/` | 路由白名单、双 JWT、JSON 严格校验 |
| 业务核心 | `internal/core/` | 所有业务逻辑 + SQL + 数据库迁移 |
| OpenAPI | `internal/contract/` | 从代码生成 API 文档（`api/openapi.json`） |
| 模拟器 | `internal/simulator/` | 测试/开发用的内存版节点 + 凭证签发 |
| 集成测试 | `integration/` | 对真实 PostgreSQL 跑端到端测试 |
| 入口 | `cmd/` | `server`（服务端）、`cloudctl`（CLI）、demo 等 |

## 领域词汇表

| 词 | 含义 | 类比 |
| --- | --- | --- |
| **tenant** | 组织 / 公司，一切资源的边界 | 一个 GitHub Organization |
| **user / membership** | 人 / 人与组织的关系（含角色 admin/member） | 组织成员 |
| **project** | 一个开发环境，绑定一个 Git 仓库 | 一个 repo 的云开发环境 |
| **workspace** | project 下的一个工作区（main 或 isolated） | 一个分支的 checkout |
| **operation** | 持久化的异步任务（建环境、启停沙箱…） | 一个可重试的后台 job |
| **issue** | 看板上的一张卡片 | Jira/Linear 的一个 ticket |
| **issue_status** | 看板的一列（可自定义） | Trello 的一列 |
| **label / subscriber / comment / view** | 卡片的标签 / 关注者 / 评论 / 保存的视图 | 同名概念 |

## 它和 Multica 的关系

Issue 看板能力迁移自参考项目 Multica，但**完全按 Ora Cloud 自己的架构重写**（不是抄代码）。
三次迁移：第一波做了核心看板，第二波补齐了评论/标签/订阅/编号/搜索/批量等周边，第三波把正式
Web 前端迁入 [`frontend/`](../../frontend/README.md)。协作交互的决策见
[Issue 协作 ADR](https://github.com/ora-space/specs/blob/main/decisions/cloud/issue-collaboration/0-typed-targets-and-explicit-execution-intent.md)。

## 下一步读什么

| 我想… | 去读 |
| --- | --- |
| 知道系统为什么这样设计 | [Cloud ADR](https://github.com/ora-space/specs/tree/main/decisions/cloud) |
| 跑起来看看 | 根目录 README / `scripts/demo-issue-board-web.sh`（看板演示）· `frontend/`（正式前端） |
| 改代码、加功能 | [../agent/architecture.md](../agent/architecture.md) → [../agent/adding-features.md](../agent/adding-features.md) |
| 查某个接口的字段 | [../agent/api-reference.md](../agent/api-reference.md) |
| 查某张表的结构 | [../agent/database.md](../agent/database.md) |
