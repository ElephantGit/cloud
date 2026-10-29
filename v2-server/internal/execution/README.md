# 公共执行端口

`execution.Service` 是 chat、工作流和定时任务共用的进程内 Go 接口草案。
当前只有类型、文本输入校验和 `executiontest.Stub`；没有真实 runtime、SQL 或 HTTP 装配。
替身默认返回 `ErrUnavailable`，不会生成“Agent 已完成”的结果。

## 两条执行路径

- `Start`：有项目，选择预置 Project/Workspace 和 `ExecutorID`，经真实 Node 启动 Agent。
- `StartModel`：无项目，直接通过 M2 provider 调模型，只有 `ModelID`，不启动 Agent、不创建 Workspace、不提供工具。
- 两者共用 `Append`、Ref、Result、会话订阅、取消和历史读取。Append 沿用 session 的模式和绑定。
- `Prompt` 是 ACP ContentBlock 数组；首版只接 text，其他块明确返回 `ErrUnsupported`，空/畸形输入返回 `ErrInvalidRequest`。
- `Catalog.ListExecutors/ListModels` 从 M2 的实际配置和已安装产物提供 ID/显示名，包含 fixture；不列虚假的可执行项。
  M3 负责 Project/Workspace 的准备、契约列表/读取接口，以及会话内目标、执行器和模型的不可变快照。
  清单更新不能改写历史选项，快照只含公开信息与绑定版本，永不含密钥。

## 身份、接收与忙碌

Scope 来自已认证的页面服务，或后台持久任务的归属；后台不要求浏览器在线，不保存 cookie。
执行侧每次检查当前成员关系和资源归属。Caller 的 kind/ID/node/容器轮次路径/attempt 在 Scope 内定位一次调用。
chat 的 ID 是 conversation，workflow 是 run，schedule 是 occurrence；轮次路径从外到内，含容器 ID 和零基轮次。

接收和 Workspace 排他是一个原子步骤；跨 chat/workflow/schedule 生效，只按 Workspace，不按整个 Project。
先查询同身份：相同请求返回原 Ref，变更参数报 `ErrConflict`；之后才检查 Busy。
`ErrWorkspaceBusy` 明确表示未接收、不会稍后自己启动，没有公共队列；等待策略由页面模块负责。

接收方法返回后不能在后台迟到接收。context 截止先赢则返回 context 错误；接收先赢则返回 Ref。
存储提交结果无法确定时返回 `ErrAdmissionUnknown`，调用方用完全相同的请求重试核对，不能换新 attempt。
`ReadAttempt` 的一次未命中不能终结一个仍在途/结果未知的 Start。业务显式重跑才用新 attempt。

## 会话事件和用量

`Subscribe(sessionID, afterSequence)` 用 session 内连续序号，先回放再跟随，没有交接空档。
Event.Ref.TurnID 对轮次外事件为空；保留 ACP SessionUpdate 原始 discriminator、元数据，以及 ACPSessionID，
子代理不同 ACP session 的事件不会被误丢。模型直连使用 `text_delta`，不伪造 ACP tool 或 Agent 活动。
`Result.Usage` 保存 ACP PromptResponse.usage；模型直连将 provider 用量转为相同 ACP Usage 结构，没有则留空。

`turn_ended` 不关闭订阅，后续 usage、其他轮次继续可见。只有 `session_ended` 后 EOF；游标已确认它时直接 EOF。
工作流/定时调用方按 TurnID 过滤，收到目标 turn 终态后主动 Close 观察流，不等待 session EOF。
关闭观察流或其 context 不取消执行；超出历史的游标报错，读取故障不能冒充 EOF。
最终可见回答、停止原因、失败码、用量分别保存，thought/tool 输出不拼入 Answer。

## 停止、释放与恢复

Cancel 等执行侧停止访问目录和终态写入；与自然完成竞争时返回实际结果。
超时或 `ErrStopUnconfirmed` 都不能表示目录已释放，也不能据此派发后继执行。
确认 quiescence 后才能发布 turn 终态并释放 Workspace；End 只清理本 session，不能误停后来占用同目录的 session。
清理失败单独返回，不改写已经持久化的回答或结果。

End 后仍可读取 Session、Turns、Attempt 与事件。重启将旧活 session 设置 EndedAt/session_ended，
活动 turn 写 interrupted/turn_ended，完成后才开放读取；启动前清理残留，清理未确认的 Workspace 不重新接收。
不恢复旧活会话、不自动重放 prompt；对已结束 session 的 Append 返回 `ErrSessionEnded`。

首版 Agent 为完全访问模式：适配层在受控演示环境中选 ACP allow_always，只有 allow_once 时选它；
没有允许选项则取消该权限请求并明确失败，不悬挂等待页面。权限回应 UI 后置，模型直连不提供工具。
附件、动态插件、模型热切换、任意仓库导入和新建 worktree 不由本草案宣称已支持。
页面拥有标题、排序、删除、业务队列及自己的 HTTP/SSE；公共层拥有运行和记录。

## 测试边界

callback 替身用于消费方测试，不实现调度、幂等、回放、持久化或进程生命周期。
本包测试直接验证文本输入的保真与拒绝，以及未配置替身的不可用结果。
上述运行约定必须由真实 runtime、PostgreSQL/race 与 Node+Agent+fixture 测试兑现；真实 provider 单独验收。
