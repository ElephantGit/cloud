# 公共 Agent 执行端口

`execution.Service` 是 chat、工作流和定时任务在同一进程内消费的 Go 接口草案。
当前只有类型和 `executiontest.Stub` 测试接缝，没有 Node 实现，不注册 HTTP 路由，
没有装配到 `main.go`。替身默认返回 `ErrUnavailable`，不会生成“Agent 已完成”的演示结果。

## 身份与会话

- `Scope` 由已认证的页面服务构造。实现需要检查租户成员和资源归属，不能直接信任 HTTP body。
- `Caller.Kind + ID + NodeID + Rounds + AttemptID` 在 Scope 内标识一次调用；
  ID 分别表示 conversation、workflow run 或 schedule occurrence。
  工作流的 Rounds 是外到内的容器 ID/零基轮次路径，避免嵌套循环和兄弟容器串记录。
- `Start` 返回 session/turn 两个 ID；`Append` 在相同 session 中追加新 turn。
  chat 的排队由页面负责，执行服务遇到同 Workspace 的活动回合返回 `ErrWorkspaceBusy`。
- 同 attempt 的完全相同请求重试返回原 Ref；参数不同返回 `ErrConflict`。
  主动重跑必须使用新的 attempt，不重放 interrupted 回合。
- 首个 runtime 要求 ProjectID，WorkspaceID 留空选择项目的主 Workspace；
  已有独立 Workspace 可显式传入。项目、Workspace 的准备与列表由公共项目服务负责。
  无项目运行、任意仓库导入、新建 worktree、附件、动态插件选择、权限回应和模型热切换
  尚未在本接口交付；页面不能据此宣称这些能力已可用。

## 事件与结束

`Subscribe(ref, afterSequence)` 从游标之后回放，再无缝跟随实时事件。
序号在一个 turn 内从 1 连续递增；超出已有历史的游标报错。终态事件送达后返回 `io.EOF`；
游标已经确认该终态时直接返回 EOF。
读取失败不能用 EOF 冒充正常完成。订阅关闭或请求 context 取消只停止观察。

ACP `SessionUpdate` 原样保留 discriminator、扩展元数据与内容，文本、思考、工具、plan、usage
由原有页面归约器区分。`Result.Answer` 是完整可见回答，`StopReason` 单独保存；
调用方不能把思考、工具输出或停止原因拼进回答。失败与取消可以带部分回答，是否作为领域输出
由页面决定（例如工作流只有成功完成才继续下游）。

`Cancel` 等执行侧停止确认和终态写入；与自然完成竞争时返回实际结果。
超时或 `ErrStopUnconfirmed` 都不是“已经停止”，调用方通过历史/订阅继续核对，不能立即重开同 Workspace。
`End` 结束整个 session 并确认清理；单轮完成不代表 session 已关闭。

`ReadSession` 读公共元数据，`ReadTurns` 分页列出 prompt/结果，`ReadAttempt` 精确查询一个调用。
历史事件继续通过 `Subscribe` 读取。会话标题、排序、删除和页面的 HTTP/SSE 协议属于页面服务。

## 测试边界

消费方可注入 `executiontest.Stub` 的函数模拟成功、busy、失败、停止超时及历史读取。
函数自行控制同步和返回值；替身不实现调度、幂等、持久化、事件回放或进程生命周期。
这些接口承诺仍需真实 runtime 的数据库、race 和 Node + Agent + fixture 端到端测试兑现。
本包测试和示例只证明消费方式与未配置时失败，不是上述能力的验收。
