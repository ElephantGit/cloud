# Node 本地 IPC

此包实现真实 Node v1 的 Controller 侧传输：Unix socket、四字节大端长度、`0x01` 标签、
16 MiB 帧上限、Hello 身份/版本/能力校验、串行写入、双向 heartbeat、显式事件 ACK 和关闭。
协议依据 desktop `cab59a3c0ecd4aa31b939f29a5fca76d03a4e64e` 的 `crates/node-protocol`。

`Dial` 成功后由调用方持有 Client，始终有一个 Receive owner；Send/Ack/Close 可并发。
每帧读写有截止时间；传输过程中取消或出错会关闭连接，禁止复用半帧。Close 等 heartbeat goroutine 退出。
`Options` 来自后端部署配置，socket 位于该 Workspace 的私有目录，不能从页面请求指定。

Receive 保留 payload、operation/execution/sequence 与 ACP 扩展信息。**不会自动 ACK**：
运行时先处理并持久化事件，再 Ack 原始坐标；发送成功不等于 Node 已接收业务命令或执行完成。
Node 执行流序号与页面的 session 事件序号是两种坐标，后续持久化适配负责对应。

## 验证

`go test -race ./internal/node` 包含真实 Unix socket fixture：延后 ACK、心跳、身份拒绝、
帧损坏/截断、短写、取消半帧、Close 解除阻塞和重复关闭。

真实二进制测试需另设 `V2_NODE_TEST_BIN_DIR`，目录内有未修改的 `ora-node`、
`ora-process-host`、`ora-process-guardian`，从上述固定源码以 Rust 1.95.0 构建：

```sh
cargo build --locked -p ora-node -p ora-process-host -p ora-process-guardian
V2_NODE_TEST_BIN_DIR=/absolute/desktop/target/debug go test -race -run TestRealNodeHandshake -v ./internal/node
```

测试在原生 Linux/WSL home 下建立短的私有临时目录，结束后删除它。home 必须位于
ext4/xfs/btrfs：host 会拒绝容器 overlay、Windows 挂载和过长的 guardian socket 路径，不能跳过此检查。
容器可编译测试二进制，然后在 WSL 原生执行；未设置二进制目录时该测试明确 skip，不能算真实验证通过。

## 边界

目前只交付传输，尚未装配进 v2-server。真实边界测试覆盖 Hello、两个 Node heartbeat 和
空闲 Node/host 的 SIGTERM 正常退出；不代表已验证 Agent 工具执行后的 guardian 清理。
固定资产准备命令、Workspace 进程监督、启动前残留回收、clone/插件/Agent 命令编排、Git/模型服务、
数据库 ACK 顺序与 M4 适配由后续运行时实现承接。低层连接取消不能直接绑定到页面观察请求。
