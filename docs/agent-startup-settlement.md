# Agent 启动结算

[中文](agent-startup-settlement.md) | [English](agent-startup-settlement.en.md)

首条 prompt 与启动工作项只证明 Cloud 声明了意图。派发登记意味着 Node 可能已经执行，即使尚未收到 ACK 或 Thread 记录。启动对账据此决定能否撤回；不会以缺回复推断执行不存在。

此实现用于评审 specs 的 `20261012-bounded-agent-startup-and-undispatched-withdrawal` 提案；新终态边界与默认预算须先由 owner 批准并同步核心用例，随后才能合并用于部署。没有数据迁移，也不重写历史 Node 结果。

## 未登记的启动

在与派发登记相同的事务锁下，确认不存在 Agent 执行、已绑定工作项或已投递命令，才能撤回启动工作项与未投递命令。撤回、Thread 终结、queued 轮次丢弃、业务结算和 owned delete 声明同事务提交；失败全部回滚，迟到登记被拒绝。

| 条件 | 业务结果 | 安全原因 |
|---|---|---|
| 普通结束或取消 | `cancelled`，交付 `skipped` | 无失败原因 |
| 冻结输入要求模型代理，但当前已初始化、连接中的 Node 明确不具备能力 | `failed`，交付 `skipped` | `model_proxy_unavailable` |
| 启动预算耗尽 | `failed`，交付 `skipped` | `agent_start_failed` |

普通结束接口仍返回 `202/ending`，由周期对账结算，不改变同键重放语义。Project 操作槽忙时，Run 保持 `releasing`，由已有 delete 重声明循环继续清理。

## 已登记且尚无启动记录

预算耗尽后，Thread 保持 `ending`，GET Thread 返回 `failureCode=agent_start_failed` 并拒绝新输入；工作项、登记、queued 轮次、命令与 Workspace 全部保留。已有用户结束原因保留，否则声明一个策略取消。重复扫描不增加结束命令。

迟到记录按原顺序接管，首条记录不会重新打开 `ending`。真实 Node 终态仍是交付的入口；交付结算后才声明删除。策略超时取消最终为 `failed`，此前接受的用户结束保留其业务结果，Node 失败继续使用受限安全原因。Cloud 不伪造 Node 结果、收据、历史或 Revision。

## 预算与验证

`issue_runs.agent_start_timeout` 默认 `30m`，可用 `CLOUD_ISSUE_RUNS_AGENT_START_TIMEOUT` 设置正时长。数据库时间以当前阶段的稳定起点判断：声明前用 Workspace 创建完成时间，声明后用工作项创建时间，登记后用执行创建时间。用户输入、请求重放和连接恢复不续期。

数据库回归覆盖结束撤回、明确能力拒绝、超时、派发竞争、迟到登记、重复结算、事务回滚与 Project 忙后的清理重试；HTTP/gRPC 回归覆盖同键重放、关闭输入、公开安全原因、迟到记录、终态与交付。真实供应商对话和管理强停仍需各自的完整验收；超时本身不能证明已经登记的执行停止。
