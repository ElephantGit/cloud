# 任务对话

[English](README.en.md) | 中文

任务级对话读取、发送、待交接消息及 Agent 选择界面。一次 Run 的 Thread 内容仍由 `issues/thread` 提供；本模块不拥有执行状态或 Node 连接。

| 文件 | 职责 |
| --- | --- |
| `api.ts` | 任务消息双向分页、发送、停止、取消/重试、恢复/放弃未交付成果 API 与安全错误文案。 |
| `conversation-panel.tsx` | 聚合任务评论、Agent Run 与待交接消息，显示历史和当前协作。 |
| `conversation-composer.tsx` | 单一 Agent 目标选择、显式提及菜单、Agent 消息/仅评论模式及交接提示。 |
| `use-conversation-draft.ts` | 一份任务草稿及历史引用，切换 Agent 只改变输入目标。 |
| `task-stop.tsx` / `task-recovery.tsx` | 当前执行停止、持久暂停和保存/放弃后的明确恢复，独立于历史浏览。 |
| `*.test.ts(x)` | MSW 与组件场景，覆盖终态续聊、切换、失败与队列。 |

依赖 `src/api` 生成客户端、`features/issues/api`、`features/issues/thread`、`features/spaces/api`、`features/conversation-content` 和 `components/ui`。只使用服务端授权后的目标名册；选择器和 `@` 菜单只在明确选择后改变接收者，普通文本中的 `@名称` 不参与路由，本地选择也不触发执行。历史选中 Run 只读，输入框始终把新消息发往任务的当前目标。分页按服务端 seq 合并，响应丢失后的重试保留同一个幂等键。
