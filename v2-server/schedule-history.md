# 定时触发历史 HTTP

候选契约为 [schedule-history.openapi.json](schedule-history.openapi.json)。与配置契约独立，避免更改配置客户端；不修改正式 Cloud OpenAPI。三条路由均使用现有登录 cookie：

| 请求 | 响应 |
|---|---|
| `GET /api/v2/tenants/{tid}/schedule-configs/{id}/runs` | `{items, nextCursor}`，空列表为 `[]`，末页游标为空字符串 |
| `GET /api/v2/tenants/{tid}/schedule-configs/{id}/runs/{oid}` | 单条持久 occurrence，包括冻结配置、状态、原因、执行引用、释放标记与版本 |
| `GET /api/v2/tenants/{tid}/schedule-configs/{id}/runs/{oid}/result` | 公共执行结果、原 Session/Turn ID、执行模式、目录标识及轮次/会话结束时间 |

每次读取检查当前成员关系、用户/租户有效性及配置创建者归属。详情同时校验任务与记录关系；无权限、任务或记录不存在均返回 `404 not_found`。缺失/失效登录 cookie 为 `401 unauthenticated`。历史存储未注入为 `503 schedule_history_unavailable`，存储错误为 `500 internal`，不向客户端返回数据库错误。历史响应禁止缓存。

## 分页

`limit` 默认 20，范围 1–100；`cursor` 应使用上页原样返回的值。游标绑定租户、用户和任务；非法编码、跨作用域游标、重复或未知参数均为 `400 invalid_pagination`。详情不接受分页参数。

排序沿用记录底座的 `recorded_at DESC, id DESC` 时间倒序，同记录时间按不可变 UUID 降序兜底；游标编码当前作用域与上页末项的 `(recordedAt, id)`。单页授权和读取处于同一个只读数据库快照，用额外一条记录判断是否有下一页，不返回总数。各页是不同数据库快照；翻页期间新增记录处在当前游标之前，刷新首屏开始新遍历。状态更新不会改变 `recorded_at`、`id` 或排序位置。游标只是位置，不是授权凭证；篡改位置不会绕过数据库权限检查。

## 状态与执行边界

接口原样返回当前 occurrence 底座的 `recorded`、`starting`、`reconciling`、`running`、`cancelling`、`succeeded`、`failed`、`cancelled`、`interrupted`、`skipped`。`reconciling` 表示需按原 Caller 核对，原因是 `lookup_required` 或 `server_restarted`，不是待重派队列。新底座将旧 `admission_unknown` 升级为该状态；旧 `workspace_busy/admission_not_accepted` 原因分别升级为 `execution_busy/execution_not_started`。`execution_busy` 仅表示同会话或运行的上一轮未完成；同任务重叠仍记录 `previous_unfinished`。升级保留身份、冻结配置、引用与占位，HTTP 只读升级后的值。`recorded` 仅表示触发已记录；`released` 表示该记录不再占用同任务执行资格，不能当作成功证据，也不代表目录已删除。结果终态仍可能是 `released=false`，需等待执行侧停止/结束确认。跳过原因与状态分开提供。

读取不会调用 Start、StartModel、Append、Cancel、End、Subscribe 或任何状态写入。配置接口的 `runs` 字段仍为空，历史需通过本接口单独读取。

## 公共结果

`/result` 不接受查询参数，也不接受客户端指定 Session/Turn/Caller。先按当前登录用户、租户、配置创建者和 occurrence 归属鉴权；再从保存的 occurrence ID 得到调度 Caller（Kind=schedule，ID=AttemptID=occurrence ID），从 `executionRef` 解码已保存的公共 Ref。经 `Options.Execution.ReadAttempt` 读取并核对完整 Caller（包括空 NodeID/Rounds）和 Ref，再用相同 Scope 调 `ReadSession` 核对 Session ID。公共服务也须按契约重新检查当前权限。该入口只适用于这套调度 Caller 绑定，不能拿来读取工作流节点 Caller。

列表与 occurrence 详情不访问公共服务，不因公共服务故障而隐藏原触发记录。`/result` 缺少已存 Ref、公共记录不存在或无权读取时返回 `404 execution_result_not_found`；服务缺失或不可用返回 `503 execution_unavailable`；Ref/Caller/Session 错配返回 `502 execution_reference_mismatch`；应有终态结果却缺失或状态非法返回 `502 execution_result_incomplete`；其他公共读取错误返回 `502 execution_read_failed`。错误响应不携带未验证结果或内部错误文本。

`result=null` 只表示尚无终态的活动轮次，不是成功；已结束的 Session/Turn 或 occurrence 终态缺失结果时不能这样返回。存在结果时完整保留 `status/answer/answerComplete/stopReason/failureCode/usage`，不截断回答，不从事件拼凑结果，不把缺失 usage 补成零。**HTTP 200 仅表示读取成功：执行成功必须同时满足 `result.status=completed` 和 `result.answerComplete=true`**。公共方返回的部分或不完整回答保持原值，不能仅凭 occurrence 的 `succeeded` 展示为成功。

`endedAt` 是轮次结束，`sessionEndedAt` 是会话结束。Agent 的活动轮 Cancel 会结束会话；纯模型 Cancel 只停当前轮，不能仅凭 cancelled 推断会话结束。End 保留历史和目录，`workspaceId` 是目录身份而非目录路径或存在性保证。重启后读取仍使用原 Caller/Ref，不恢复追问、不重放执行。HTTP 不自动读取整段工具事件；完整最终回答与事件投影是不同能力。

真实 PostgreSQL HTTP 测试校验候选契约、全部既有状态及释放标记、分页/游标隔离、当前权限撤销、服务不可用和错误脱敏。公共 Stub 测试另验证完整长答案、Caller/Ref 错配、部分结果、两种 Cancel 会话边界、结束后读取和 GET 零写入；Stub 不证明真实执行或公共持久化。

有公共 `internal/executionruntime` 与调度编排实现的 Linux 组合源码可显式运行纯模型跨进程验收；Agent 路径还需要公共 `agentruntime`、`projectrepos`、Node 恢复代码和准备好的固定资产：

```sh
TEST_DATABASE_URL=... go test -race -count=1 -tags execution_integration ./internal/server -run '^TestScheduleResultRuntimeRestart$' -v
V2_NODE_ASSETS_DIR=... TEST_DATABASE_URL=... go test -race -count=1 -tags execution_integration ./internal/server -run '^TestScheduleAgentResultRuntimeRestart$' -v
```

两条测试均使用公共默认 fixture、真实 Service 和 PostgreSQL。第一子进程真实执行，确认完整结果和 End 后，经 HTTP 读取列表、单项与完整结果；完全退出后，第二子进程重新打开数据库与服务，比较三种响应中的原身份、冻结内容、状态和结果是否保持一致。每次 GET 前后比较 occurrence，证明读取不推进状态。Agent 另核对实际 shell 工具文件在 End 和新进程后仍存在、内容正确，不能只凭 Agent 回答判断工具执行。

测试使用短的独立 Linux 目录容纳 Node 套接字，父进程等两个子进程结束后才清理；没有共享内存、持久替身或自建 provider。缺少实际实现或资产时不能用 Stub 顶替；普通 CI 不开启该 tag。此证据验证成功运行的只读持久化边界，不替代浏览器页面、主干完整启动或公共驱动异常恢复验收。
