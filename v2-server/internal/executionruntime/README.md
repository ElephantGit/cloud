# 公共执行服务装配

本包将 `execution.Service` / `Catalog` 连接 PostgreSQL、`modelturn` 和项目 Agent driver。
调用方传入可信配置；接收和公共读取检查当前成员关系，已经接收的 worker 按原归属保存终态。
页面 HTTP 接口由领域服务接入。

## 外部 provider 验收

Linux 测试 `TestConfiguredProviderServicePostgresRestart` 使用真实公共 Service、专用 PostgreSQL
schema、`SeedDemo` 的独立 Git 项目，以及固定 Node/OpenCode。它验证无项目短文本回答和有项目
工具写入固定 marker，随后 End、关闭并重新打开连接池和 Service，比较持久化结果与完整终态。
模型回答只要求 completed、AnswerComplete 和非空，不要求固定文案。

默认明确 skip，表示**外部 provider 未验收**。只有 `V2_REAL_PROVIDER_TEST=1` 才会发真实请求；
显式开启后，缺少任意 `V2_MODEL_BASE_URL`、`V2_MODEL_API_KEY`、`V2_MODEL_NAME`、
`TEST_DATABASE_URL` 或 `V2_NODE_ASSETS_DIR` 都直接失败，不退回 fixture 或跳过。
provider 必须是 `ModelsFromEnvironment` 支持的 HTTPS OpenAI 兼容接口，并支持固定 OpenCode 的
工具调用。配置应由本地安全环境注入，不把密钥写入命令、提交或验收记录。

在仓库根目录运行 `task v2:node:prepare` 准备已校验资源。需要原生 Linux/WSL x86_64、Git、
`git-http-backend`、Go（版本见 `go.mod`）、可创建/删除 schema 的测试 PostgreSQL，以及可直连
provider 的网络；当前 RemoteProvider 不使用环境代理。HOME 必须位于原生 ext4/xfs/btrfs
短路径，不能使用 Windows 挂载或容器 overlay。测试资源目录和执行目录的约束见
[`node/README.md`](../node/README.md)。

上述变量已经安全注入环境后，在 `v2-server` 运行：

```sh
V2_REAL_PROVIDER_TEST=1 go test -race -count=1 -timeout=12m \
  -run '^TestConfiguredProviderServicePostgresRestart$' -v ./internal/executionruntime
```

也可以在构建容器里 `go test -race -c -o /output/executionruntime.test ./internal/executionruntime`，
再在原生 Linux/WSL 文件系统运行该二进制，传入同名 `-test.run`、`-test.v`、`-test.timeout=12m`。
容器编译无需 provider 配置或密钥。

测试入口不自动重试失败请求或整轮；固定 OpenCode 保留自己的模型请求与工具调用策略，因此
一次 Agent 轮次可能产生多次模型请求；是否收费以所选 provider/model 为准。成功与失败输出都不打印
provider/数据库 URL、密钥、回答正文或原始 Node 日志。模型输出会暂存在本次专用 schema 和私有目录内；成功确认自有进程停止后，清理
这些测试数据，不使用服务的 `V2_DATA_DIR` 或已有项目。停止未确认时失败并保留独立证据，避免删除
活跃运行目录。此入口不验证页面路由，也不把默认 skip 或仅编译通过算作外部 provider 验收成功。
