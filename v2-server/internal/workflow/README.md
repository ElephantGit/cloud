# 工作流执行内核

本包接收冻结的图、输入、每个 Agent 节点的 executor 映射和角色提示词快照，通过应用现有的
`execution.Service` 执行。它不依赖正式 Cloud 的数据库、分布式调度或 Node 控制平面，也不自行
创建第二个 execution runtime。包本身不注册 HTTP 路由；SQL、身份校验、资源目录解析和后台
worker 生命周期由调用方负责。定义和发布快照已由 `internal/store/workflows.go` 持久化，
`internal/server/workflows.go` 提供对应 HTTP 接口。`Host` 与 `internal/store/workflow_runs.go`
接通运行/attempt 持久化；`main` 在公共执行层恢复之后创建 Host，并在关闭执行层前等待其退出。
运行准入的 HTTP、目录绑定解析和前端数据适配仍待接入，尚不能从页面启动真实运行。

## 代码复用来源

`workflow_*.go` 及同名测试来自同仓库 `internal/core` 的 `0d3299d5` 版本；文件头记录具体来源。
仅修改包名、隔离数据库入口，并将循环和迭代里的 Agent 启动改为传入回调。
v2 的改动限制在自己的目录，因此不移动正式后端的文件，也不让临时模块导入整个正式 core。

| 语义 | 复用代码 |
| --- | --- |
| 发布校验、变量类型、文件引用 | `workflow_graph.go`、`workflow_variable_value.go` |
| 选择器、模板与条件运算 | `workflow_condition.go`、`workflow_condition_template.go` |
| 分支传播、非活跃节点、同步节点 | `workflow_scheduler.go` |
| 聚合与最终结果绑定 | `workflow_aggregator_graph.go`、`workflow_output.go` |
| 容器边界、循环、迭代与轮次归档 | `workflow_scope.go`、`workflow_loop_*.go`、`workflow_iteration_*.go` |
| 重试策略与状态大小上限 | `workflow_retry.go`、`workflow_state_budget.go` |

保留原用例验证 null、false、0 的已赋值语义、混合模板、分支不执行、作用域隔离、退出条件、
轮次记录及损坏状态拒绝。原来依赖正式数据库的 iteration-entry 用例改用同一图和断言驱动
`Machine`；HTTP fault 的断言改为验证相同的错误参数，HTTP 状态码由调用方负责。

## 调用与持久化约束

`Validate` 供发布和运行准入使用，未完成草稿不需要通过它。`NewMachine` 复制冻结输入并拒绝
缺失的 executor 映射或角色快照，文件类型全局变量沿用原来的规范化规则。

`Run` 在调用方拥有的 worker context 中串行运行，每次状态变化调用 `Commit`。`Commit` 必须
持久化后才返回成功；带有执行 receipt 的 checkpoint 还必须保存到节点历史，即使后续 checkpoint
已经清空 Active。存储失败会阻止后续外部执行。取消 worker 时，用 `ErrUserCancelled` 作为
context cause 表示用户停止；其他取消表示进程中断。

一个 run 的每个节点、容器轮次和重试均有独立的 caller identity。Agent 完成后，先确认 `End`
成功，再提交最终回答并进入下个节点；停止未确认时保留运行状态及 receipt，不释放执行顺序。
只接收匹配 turn 的权威最终回答，流式 history 和其他 turn 的事件不能进入变量池。观察断流时
读取同一 attempt 的最终记录，不能靠重新执行恢复。进程重启后的未完成 run 应由调用方标为
interrupted；本包没有从 checkpoint 自动重放的入口。

保持既有重试默认值和条件：未配置时最多重试两次，初始等待十秒，只有 `agent_failed` 重试。
公共执行结果未明确提供该失败码时保留其原始失败，不根据普通失败或断流猜测可重试性。
循环最多 100 轮，不支持嵌套复合容器。内核限制 256 个节点、2048 条边，并对无容器图也施加
总状态大小上限。执行结果超过预算时拒绝该次状态写入，不保留过大的回答。

## 定义与发布快照

定义和快照继续使用 `api/openapi.json` 中的创建、读取、更新、归档、发布、历史和恢复接口，
认证沿用 v2 的 `ora_session`。草稿可以不完整，发布才使用本包的图校验与文件变量规范化。
存储图文档保留 JSON 数字精度；不会把编辑器注释或视口字段投影掉。发布不修改草稿，恢复
只替换草稿图并递增文档版本；快照版本按工作流独立递增。

存储适配沿用 `internal/core/workflow.go` 的领域行为。原文件绑定正式 `transaction` 和
`PublicRequest`，因此这里改接现有 pgx 连接池及成员校验。更新、恢复、归档、发布共享行锁，
并发修改用文档版本拒绝覆盖；活跃名称唯一约束在归档后释放名称。创建、发布和归档的幂等键
与完整响应在同一事务提交，重放前重新鉴权，之后才判断资源/版本。同键异参返回冲突。

## 验证

`go test ./internal/store ./internal/server` 包含真实 PostgreSQL 的并发、权限撤销、归档、
快照隔离和关闭重开验证，以及正式 OpenAPI 响应验证；必须设置 `TEST_DATABASE_URL` 才会
执行数据库用例。未配置数据库时跳过这些用例，不能据此判断持久化验证通过。

`go test ./internal/workflow` 执行迁移用例和 execution port 的适配回归测试。
新增用例覆盖两个 Agent 串行传值、先持久化后执行、session 停止确认、取消、断流、响应丢失和
不完整回答拒绝。这些测试使用明确的 `executiontest.Stub`，不能替代真实 Node/PostgreSQL 集成验证。
仓库发布门禁仍是带 `TEST_DATABASE_URL` 的 `go vet ./...` 和 `go test -race ./...`。

## 运行记录与 worker

`Admission` 是应用内部的命令，调用方必须先从公共目录解析 executor 和角色快照，不能把它当作
HTTP body 直接解码。存储只从指定已发布快照加载图，并检查项目属于当前租户；它复用定义接口
的幂等事务。`Record` 也不是正式 API 的响应 DTO：它包括内部冻结定义、检查点和写入版本。
运行内容按发起者隔离，与公共执行接口的会话归属一致；共享工作流文档不扩大会话访问权限。

`Host.Launch` 先读取持久运行并原子领取 pending 行。幂等创建返回的是原始响应，不能据此重建
运行中的 Machine。worker context 属于进程，HTTP 断开不会取消已接收的运行。每次检查点按
revision 比较更新，旧 worker 不能覆盖新状态；终态只读。每个 attempt 在 Start 前保留完整
Caller，收到 Ref 后只能补写同一引用，清空 Active 也不删除历史；事件和回答不另建存储通道。

取消只先记录请求时间，不把“请求已接受”当作“执行已停止”，也不改变 worker revision。
停止未确认时保留 running 和 receipt；后续取消可按同一 Caller 核对并确认 End，再结算图状态。
启动恢复只在公共运行时完成全局恢复后执行，包含失去成员身份的旧运行。未完成运行按既有
failed/interrupted 语义结算，已请求取消的记为 cancelled；从不重放 prompt。缺失响应的历史仍
保留 Caller，可经公共 ReadAttempt 找回，不能据此生成替代 attempt。

Host 的 PostgreSQL 集成用例覆盖并发 Launch、请求断开、串行传值、收据分页、关闭等待和
停止未确认后的核对。这些用例的执行端口仍是替身，不代表真实 Node/工具产物链路已验收。
