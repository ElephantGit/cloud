# 定时任务执行状态

`ApplyOccurrenceEvidence` 只由可信执行适配层调用，浏览器不能提交状态证据。公共执行的 Session、Turn、结果和事件由 execution.Service 保存，本层只保存触发身份、冻结配置、状态与引用。

状态枚举为 `recorded`、`starting`、`reconciling`、`running`、`cancelling`、`succeeded`、`failed`、`cancelled`、`interrupted`、`skipped`。

| 当前状态 | 事实/操作 | 下一状态及 reason |
|---|---|---|
| recorded | begin，配置有效且同任务无占位 | starting，占用任务执行资格 |
| recorded / 新触发 | 同任务已有占位 | skipped / previous_unfinished |
| recorded | 配置版本变化或自动触发已暂停 | skipped / configuration_changed 或 paused |
| starting | 公共服务明确拒绝 | skipped / 拒绝原因，释放占位 |
| starting | 需要只读核对（调用错误或本地引用未保存） | reconciling / lookup_required，保留占位 |
| starting / reconciling | 原 Caller 对应完整 Ref | running，reason 清空 |
| starting / reconciling，无 Ref | Start 已返回或公共启动恢复已完成，原 Caller 查询未命中 | skipped / execution_not_started |
| running | 请求取消 | cancelling，仍占位 |
| running / cancelling / reconciling | 同 Ref 的真实终态 | succeeded / failed / cancelled / interrupted，仍占位 |
| 上述结果终态 | 同会话 End 确认成功 | 保留状态、reason、Ref，释放占位 |
| recorded | 启动恢复 | skipped / server_restarted |
| starting / running / cancelling | 启动恢复 | reconciling / server_restarted，保留 Ref 和占位 |

`reason` 是稳定原因码，不是异常原文。拒绝原因包括 `execution_busy`、`execution_unavailable`、`invalid_execution_request`、`execution_unsupported`、`execution_target_unavailable`，以及配置校验的 `invalid_snapshot`、`unsupported_schedule_target`、`model_required`、`project_required`、`executor_required`、`invalid_prompt`。`execution_busy` 只消费公共服务的 Busy 拒绝，不表示其他调用方抢占项目目录，不进入等待队列。成功但回答不完整转 `failed/result_incomplete`；其他执行终态使用公共 Result.FailureCode（过长时用 `execution_failed`），没有原因时为空。

`released=false` 表示本记录仍占用**同任务的执行资格**。`released=true` 表示不占位：recorded 尚未 begin、skipped 未执行，或结果终态已经确认 End。它不证明目录已删除，也不是结果完整性标志。End 保留历史及目录；模型 Cancel 只停当前轮，Agent Cancel 结束会话，定时任务两路都随后调用幂等 End。

配置行锁和部分唯一索引保证同任务最多一个未释放记录，自动与手动触发共用此约束。不同任务即使属于同一项目也不互斥；每次触发新建会话，有项目时公共服务创建目录。触发唯一键和 expectedVersion 防重复与过期状态写入，Ref 绑定后不可替换。

宿主先完成公共 Service 的启动恢复，再创建单个 Coordinator/ConversationAdapter 并调用 Recover。旧轮次的 interrupted 以公共持久记录为准；本地不会重放 prompt。适配层按 occurrence 串行处理 Start、核对与取消，尚在 Start 中的记录不能提前用 NotFound 收尾。读取错误、Caller/Ref 不匹配、已绑定记录查询缺失均保留占位；结果已保存但 End 未确认只重试 End。

已知限制：状态写入和恢复查询要求创建者当前仍是租户有效成员。创建者在运行中被移出租户时，适配层无法写入终态和 released，记录会持续占用同任务执行资格；当前没有自动解除该占位的机制。

没有10分钟等待、自动重试执行或停机补跑。跳过和手动运行均不改变原计划锚点。PostgreSQL 测试证明本层状态和占位，Stub 只证明公共接口消费行为，不能替代真实执行/目录/完整历史验收。

取消恢复前提：cancelling 经 recover 转为 reconciling 后不保留取消意图。本流程依赖公共执行在服务重启时将旧轮次结算为 interrupted，且宿主须先完成公共恢复再恢复调度；否则核对到仍运行的轮次可能回到 running。
