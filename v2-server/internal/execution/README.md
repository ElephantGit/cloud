# 公共执行端口

`execution.Service` 是 chat、工作流和定时任务共用的进程内 Go 接口。
本包只有类型、文本输入校验和 `executiontest.Stub`，真实执行、PostgreSQL 和入口装配由调用实现提供。
替身默认返回 `ErrUnavailable`，不会生成“Agent 已完成”的结果。

## 两条执行路径

- `Start`：选择 `ProjectID` 和 `ExecutorID`，后端从项目已保存的仓库 clone 对话目录，经真实 Node 启动 Agent。
- `StartModel`：无项目，直接通过共享 provider 调模型，只需要 `ModelID`；不启动 Agent、不建目录、不提供工具。
- 两者共用 `Append`、Ref、Result、会话订阅、取消和历史读取。Append 沿用 session 的模式和绑定。
- `Prompt` 是 ACP ContentBlock 数组；首版只接 text，其他块返回 `ErrUnsupported`，空或畸形输入返回 `ErrInvalidRequest`。
- `Catalog.ListExecutors/ListModels` 提供可运行的 ID 和显示名：默认 fixture，以及已配置的一个 OpenAI 兼容真实 provider。
  Agent 与模型直连共用这份配置，凭据只留在后端。未配置的模型或不可用的 Agent 产物不能列为可运行项。

项目由共享项目服务创建：同步 `clone --mirror` 仓库和指定分支，成功后保存项目及 snapshot ref。
执行端口不接收原始仓库凭据或任意目录。Session 保留目标、执行器和模型的选择快照，清单更新不能改写历史。

## 身份与目录

Scope 来自登录会话或后台任务的持久归属，不从请求正文直接取值。首版沿用 dev 演示用户和演示团队；
执行侧检查成员关系与资源归属，无权限的资源返回 `ErrNotFound`。

Caller 的 kind/ID/node/有序容器轮次/attempt 在 Scope 内定位一次调用。
chat 的 ID 是 conversation，workflow 是 run，schedule 是 occurrence；容器轮次从外到内，索引从零开始。
同一 attempt 重复提交相同请求只返回原 Ref；改换目标、执行器、session 或 prompt 返回 `ErrConflict`。
显式重跑使用新的 attempt。

`Target` 只有 ProjectID，调用方不选择 Workspace。后端按以下粒度创建目录，并将生成的 WorkspaceID 记入 Session：

- 每个 chat 会话一个目录；定时任务每次触发创建独立会话和目录。
- 同一 Scope 下 `CallerWorkflow.ID` 相同的一次工作流运行共用目录，节点串行执行。
- 不同对话和不同 workflow run 不争抢目录，即使它们属于同一个项目。
- 模型直连的 Session.WorkspaceID 为空。

接收执行只用单进程内的锁和一次数据库写入，持久记录建立后返回 Ref。
`ErrWorkspaceBusy` 仅表示同一 session 或同一次 workflow run 的上一轮尚未结束；页面自己排队，公共层不另设队列。
Start/StartModel/Append 的 context 控制本次调用，调用返回或浏览器断开不停止已开始的执行。
`ReadAttempt` 按完整 Caller 查询已有 Turn，不派发工作。

一个项目通常共用一个 Node 进程；必要时实现可改为每会话一个 Node，不改变接口。
Agent 改动不 commit、不 push，也不写回项目的仓库文件夹。End 不删除目录；演示目录由独立清理命令管理。

## 会话事件和完整结果

`Subscribe(sessionID, afterSequence)` 使用 session 内连续序号，先回放再跟随。
Event.Ref.TurnID 对轮次外事件为空。固定 Node 的 `history_snapshot` 是当前可得历史的完整替换快照：
Records 按历史位置排序，Data 保留 settled HistoryLine/preview 及可得元数据；同位置 settled 优先于 preview。
页面整体替换快照，不将 preview 与最终文本重复追加，也不直接套用 ACP 增量归约器。
HistoryRecord.Sequence 是 Node 历史位置，Event.Sequence 才是订阅游标。

快照 Incomplete 只说明过程信息缺失，不代表最终回答不完整。固定 Node 缺少原始 ACP、每轮 usage、上下文和
mode/config 更新；页面留空或显示不可用，不填 0，不标称“完整过程”。模型直连使用 `text_delta`，
不伪造 Agent 或工具活动；provider 明确报告的计数可映射为 ACP Usage，缺失时留空。

`Result.Answer` 只包含可见 assistant 文本，不混入 thought、tool 或停止原因。
`AnswerComplete` 必须经过完整读取确认，completed 必须为 true；不能仅拼接有界 preview 或 visible_text。
结果缺失、损坏或超限无法补全时，返回 failed、AnswerComplete=false 和明确 FailureCode。
StopReason 与 FailureCode 分别保留，清理失败不能改写已持久化的回答。

`turn_ended` 不关闭会话订阅；后续快照和轮次仍可见。只有 `session_ended` 后返回 EOF，游标已确认它时直接 EOF。
工作流和定时调用方按 TurnID 过滤，收到目标结果后可关闭观察流；Close 或订阅 context 取消都不停止执行。
超出历史的游标报错，读取故障不能冒充 EOF。

## Cancel、End 与重启

- `Cancel(ref)` 停止指定的当前轮次，等待执行侧确认并保存终态，保留会话供后续 Append。
  对已终态的 turn 重复 Cancel 返回实际结果，不误停之后的轮次；与自然完成竞争时也返回实际结果。
- `End(sessionID)` 结束会话，停止活动轮次并等待确认；成功后不能 Append。重复 End 幂等，历史和目录保留。
  它只影响这个 session，不能停止同项目 Node 上的其他会话。
- 超时或 `ErrStopUnconfirmed` 不代表停止成功，调用方通过记录或事件读取实际状态。
  定时任务和工作流用完 session 后主动 End，不用订阅 EOF 代替停止确认。
- 后端重启先结束旧活 session，写入 EndedAt/session_ended，将未结束 turn 标为 interrupted/turn_ended，
  再对外提供服务。不自动重放 prompt，不恢复旧会话；旧会话 Append 返回 `ErrSessionEnded`，历史仍可读。

首版只在 Linux/WSL 使用固定 Node 的完全访问模式，由 Node/Agent 管理权限；没有页面权限回应接口。
附件、动态插件和模型热切换不在首版支持范围。页面拥有标题、排序、删除、业务队列及 HTTP/SSE，
公共层负责执行和记录。

## 验证边界

callback 替身用于消费方单元测试，不实现调度、幂等、持久化或进程生命周期。
本包验证文本输入的保真与拒绝，以及未配置替身的不可用结果。
真实实现需用 PostgreSQL/race 验证重复 attempt、同会话及同 workflow run 的 Busy、Cancel/End、重启后的历史读取，
并经 M4 跑通真实 Node/Agent + fixture 的一轮对话和工具动作；真实 provider 单独验收。

固定 Node 的 IPC history 在 256 KiB 截断，visible_text 在 64 KiB 截断。完整结果从本地 sessions JSONL 只读恢复，
按历史位置去重、排序并核对指定 turnEnded，持久化成功后再 ACK。不能修改 Node SQLite 或历史文件。
超过截断阈值的回答必须在 End 和重启后从 PostgreSQL 逐字读回，丢失和超限都不能报告完整成功。
