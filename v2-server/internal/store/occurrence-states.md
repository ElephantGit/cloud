# 定时任务执行状态

`ApplyOccurrenceEvidence` 是进程内可信执行适配层调用的状态接口，不是可由浏览器填写事件的 HTTP 接口。本阶段未装配执行器、启动扫描或 UI，不能把状态测试视为模型/Node 已交付。

| 当前状态 | 已确认事实/操作 | 下一状态 |
|---|---|---|
| recorded | begin，配置仍有效且没有其他占位 | starting，占用该任务执行资格 |
| recorded / 新触发 | 同任务已有未结束占位 | skipped / previous_unfinished |
| starting | 执行侧明确 Busy 且未接收 | skipped / workspace_busy，释放占位 |
| starting | 接收结果不明确 | admission_unknown，保留占位 |
| starting / admission_unknown / reconciling | 按原身份确认接收，带稳定 Ref | running |
| admission_unknown / reconciling（无已知 Ref） | 明确未接收证明 | skipped / admission_not_accepted |
| running | 请求取消 | cancelling，仍占位 |
| running / cancelling / reconciling | 同 Ref 的成功/失败/取消/中断终态 | succeeded / failed / cancelled / interrupted，仍占位 |
| 上述结果终态 | 同 Ref 的停止与清理确认 | 保留结果，释放占位 |
| recorded | 启动恢复检查 | skipped / server_restarted |
| starting / admission_unknown / running / cancelling | 启动恢复检查 | reconciling，保留 Ref 和占位，等待公共记录核对 |

## 身份和并发

配置行锁使触发创建、begin和配置编辑串行；数据库部分唯一索引保证同一任务最多一个 released=false 的记录。不同任务不在此层互斥，Workspace 争用继续由公共执行模块裁定。手动触发和自动触发使用相同占位保护。

begin 的成功返回才允许其调用方请求外部接收，执行身份用原 occurrence ID，必须持久传递。expectedVersion 对每次状态修改做比较，阻止重复 begin 和迟到回调覆盖更新。出现冲突需重读记录核对，不能重新分配身份或将事务结果不明当作未接收。Ref 一旦记录就不可更换；后续终态和释放必须匹配该 Ref。

## 证据与恢复

`busy` 必须是执行侧明确未接收，`not_accepted` 必须是已终结原接收竞态的未接收证明。请求超时、一次 ReadAttempt 未命中均不构成该证明。`released` 必须确认执行侧停止和清理；仅收到回答、发出取消或 End 超时不能释放。状态层不替代公共执行模块判断这些事实。

宿主未来应在开启派发前遍历旧记录应用 recover，再按公共执行持久记录核对；该接口本身不在数据库 Open 时自动执行恢复，以免另一个连接池打开被误当作进程重启。已在运行的回合确认被重启打断后写 interrupted，再取得释放确认；结果已保存但尚未释放的记录保留结果和占位。恢复不会调用 Start，也不重放 prompt。

本层没有10分钟等待、取消等待或自动重试。Busy和同任务占用均留下跳过记录，显式重跑使用新触发身份。状态修改不写配置、时间规则或原计划锚点，因此不会因跳过或手动操作改变原计划。真实运行的开始/结束时间、最终回答与事件继续由公共执行记录承担，当前仅保存其稳定引用。

schema 在旧 recorded-only 表上追加字段并替换状态检查，旧记录默认保持 recorded/released=true/version=1；启动的幂等建表也适用于连接池重新打开。状态字段和占位由上述事务接口一起更新，不应绕过接口直接写 SQL。
