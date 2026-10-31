# 工作流执行内核

本包接收冻结的图、输入、每个 Agent 节点的 executor 映射和角色提示词快照，通过应用现有的
`execution.Service` 执行。它不依赖正式 Cloud 的数据库、分布式调度或 Node 控制平面，也不自行
创建第二个 execution runtime。包本身不注册 HTTP 路由；SQL、身份校验、资源目录解析和后台
worker 生命周期由调用方负责。

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

## 验证

`go test ./internal/workflow` 执行迁移用例和 execution port 的适配回归测试。
新增用例覆盖两个 Agent 串行传值、先持久化后执行、session 停止确认、取消、断流、响应丢失和
不完整回答拒绝。这些测试使用明确的 `executiontest.Stub`，不能替代真实 Node/PostgreSQL 集成验证。
仓库发布门禁仍是带 `TEST_DATABASE_URL` 的 `go vet ./...` 和 `go test -race ./...`。
