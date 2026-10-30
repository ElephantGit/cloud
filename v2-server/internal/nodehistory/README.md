# 固定 Node 的历史适配

只读 desktop `cab59a3c0ecd4aa31b939f29a5fca76d03a4e64e` 的 Node history，将
`thread_event` / `thread_preview` 适配为完整替换快照，并在指定 `turnEnded` 上恢复完整文本结果。
不修改 Node 协议、SQLite 或 JSONL；不把截断的 `visible_text`、preview 当作最终回答。

## 消费接口

在已接收 Workspace 的 Node home 上调用 `Open`，为每个 execution 创建单一消费者拥有的
`NewProjection(reader, executionID)`，将收到的两种历史消息交给 `Consume(ctx, message)`。
其他生命周期消息由 runtime 处理。Reader 的生命周期覆盖该 execution 的历史读取。

- `Observation.Snapshot` 替换整个旧快照。`Records[].Sequence` 是 Node history 位置，
  同位置只保留最新值；settled 优先于 preview。它不是页面订阅游标，也不是 ACP 原始增量。
- 新版 preview 替换全部 pending，丢弃旧版本；`Incomplete` 只说明过程展示可能不完整。
  重连 Node 后重建 projection，不能沿用旧 incarnation 的 preview version。
- 定型记录发生截断时，从本地完整 JSONL 恢复后才替换 preview。
- 终态 `Observation.Turn` 绑定消息的 `TurnID` 和确切 history `EndSequence`，
  验证本地 `turnEnded` 与停止原因一致，绝不回退到上一轮结果。
  Answer 只包含本轮父会话的文本回答，多个 settled message 用换行连接；不拼 thought、tool 或 child 输出。

消费者必须把原始 Node envelope、快照及可得的完整结果和自己的 Session/Turn 坐标持久化，
成功后才 ACK 有 sequence 的消息。preview 无 ACK，只作可替换显示。
数据库负责 Node 消息去重、session 连续事件序号、接收幂等、Workspace 排他、回放与权限检查。
此包不分配页面游标，不持久化，也不释放 Workspace。

`Consume`/`ReadTurn` 失败时，将 `FailureCode(err)` 写为明确的 failed/incomplete 结果；
不能把旧快照升级成成功。`AnswerComplete` 只有完整读取通过才可设 true。
完成一轮不表示 Node session 已停止；调用方仍按运行端口约定 End 并确认清理。

## 读取边界

读取固定布局 `sessions/<前两字符>/<再两字符>/<execution>.jsonl`，通过 `os.OpenRoot`
限制在可信的 Node home 内。execution ID 仅接受字母、数字、连字符，最长 128 字符。
按 history sequence 排序并取重复位置的最后记录；要求从 meta 到目标 turnEnded 没有缺口，
session ID、schema、时间戳和记录类型有效。只消费换行提交的记录；未完成尾行不能构造终态。

限制为每文件 64 MiB、每行 8 MiB、单轮回答 16 MiB。文件缺失、损坏、显式 gap、缺少目标终态
或超限都失败，不能静默截断成成功。超过限制的会话需要显式失败和后续产品策略。
这些限制不改变固定 Node 的 IPC record 256 KiB / visible_text 64 KiB 上限。

## 验证

单元测试覆盖超长 Unicode 回答、重复/乱序记录、多轮边界、工具数组内容、child 排除、
缺失/损坏/gap/超限、路径逃逸及 preview 替换去重。

在原生 Linux 文件系统准备 Node/Deno/OpenCode 产物并提供 PostgreSQL 后运行：

```sh
export V2_NODE_ASSETS_DIR=/absolute/path/to/prepared/assets
export TEST_DATABASE_URL='postgres://ora:ora-local@127.0.0.1:55432/ora?sslmode=disable'
task v2:node:history:test   # 仓库根目录
```

真实测试经 HTTPS clone、固定插件安装、真实 Agent shell tool 和 fixture provider 生成 792,000 字节回答，
要求 IPC 实际截断而完整读取仍逐字一致；落 PostgreSQL 后才 ACK。
随后取消第二轮、End、清理及重启 Node，删除测试用 Node history，再用全新 PostgreSQL 连接读取完整首轮结果。
SQL 部分是隔离 schema 的证据夹具；不代表页面 Session/Turn store、HTTP 或真实外部模型已经接通。
