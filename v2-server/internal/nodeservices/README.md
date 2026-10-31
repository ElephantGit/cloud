# Node 本地配套服务

`Start` 为一个 Workspace 建立两个随机 loopback 监听器：HTTPS Git/模型服务和 HTTP
插件下载。传入部署配置中的 bare Git 仓库、`git-http-backend`、若干固定包（名称、路径、SHA-256，
分别在 `/<名称>.orax` 提供）、模型 binding/provider；页面请求不能指定这些输入。固定资产由 `task v2:node:prepare` 准备。

每个实例生成独立 CA 和 Node 客户端证书，私钥只写到 StateDirectory 下新建的 0700 临时目录，
文件为 0600；Git 和 grant 端点要求该身份的 mTLS。Agent 只获得公开 CA 和短期 token。
证书有效期 24 小时，演示进程需在到期前重启；无证书轮换或正式 Cloud 的身份/租户系统。
这是本机演示的凭据分发边界，不是抵御同一 OS 用户恶意代码的沙箱。

Git 只提供单个仓库的 smart HTTP discovery/upload-pack，不允许 push、任意路径或任意命令。
请求最多 4 MiB、子进程最多 30 秒、响应最多 64 MiB；超出限制失败。它承载演示克隆，
不替代 Cloud Git Server 的交付流程。插件由启动时复制并校验的私有快照提供，Node 再校验
同一 SHA；插件 listener 没有认证，包不应含秘密，原始资产后续改动不改变正在提供的快照。

## 模型与生命周期

调用方先完成业务 admission，再 `Permit(executionID, bindingID)`，最后向 Node 发命令。
grant 只为已授权 execution/binding 签发，10 分钟到期，同一 execution 再次申请会替换旧 token。
renew 必须匹配原 execution，过期后不能续期。binding 和 Model 快照来自后端，不能由 Agent 改写。
最多同时保留 1024 个授权 execution，调用方必须在终止或启动失败后调用 `End`。

`End` 撤销该 execution 的 token 并取消活跃模型请求；它不代发 Node EndSession。
正常顺序是发送 EndSession、收齐并持久化终态、调用 End、停止 Runtime、关闭配套服务。
崩溃恢复时创建新的服务实例和凭据，不重放用户 prompt。`Close` 取消 provider、关闭监听器、
等待 HTTP handler 退出并只删除其凭据目录；实现 Provider 时必须遵守 request context。

每个 binding 声明一种协议：`openai-completions` 的 grant 指向 `/runtime/openai/v1`，token 放在
`Authorization: Bearer`；`anthropic-messages` 的 grant 指向 `/runtime/anthropic/v1`，token 放在
`x-api-key`（Claude Code 另带的占位 bearer 被忽略）。token 只在自己协议的路由上有效。

`FixtureProvider` 只替代模型：收到 shell 工具定义时返回固定工具调用，实际读取 README.md（兼容 README）和
写入 `ora-v2-result.txt` 由未修改的 Agent 执行；无工具请求返回普通 fixture 回答。
`MessagesFixtureProvider` 是同样行为的 Messages 版本：看到 `Bash` 工具时返回 `tool_use`，
收到 `tool_result` 后给出同一个回答，支持 JSON 和 SSE。
`NewRemoteProvider` 转发 OpenAI chat completions，`NewRemoteMessagesProvider` 转发 Anthropic
Messages（后端 key 放在 `x-api-key`，只透传 `anthropic-version` / `anthropic-beta`）；两者都支持 JSON/SSE，强制使用已选模型，保留成功响应的
usage 等字段。真实 API key 留在后端内存，不转发 Agent token、调用者自定义 header 或上游错误正文；
禁止 HTTP 和重定向。请求取消、grant 撤销/到期均会取消上游请求；单次远端请求上限 5 分钟。
不支持运行时模型切换或任何隐式环境变量凭据读取。

## 验证

```sh
TEST_DATABASE_URL=postgres://... go test -race ./...
V2_NODE_ASSETS_DIR="$HOME/.cache/ora-v2-node" task v2:node:agent:test
```

真实用例必须运行在 Linux/WSL 原生 ext4/xfs/btrfs 上；可在容器编译测试二进制，再在 WSL 执行。
它启动真实 Node/host/guardian，完成 HTTPS clone、固定插件安装、Agent shell 工具、回答、
第二轮流式模型请求取消、EndSession 终态、进程清理和重启后 checkout 保留。
接收到的 Node envelope 先写入并 fsync 临时日志，再 ACK；这是低层顺序验证，不替代 PostgreSQL 持久化。
未设置资产目录时明确 skip，普通 CI 通过不代表真实 Agent 用例已运行。

RemoteProvider 的测试使用真实本地 TLS HTTP 服务验证协议和取消，不是外部付费模型验收。
该包尚未装配到 v2-server 入口，也不实现公共执行端口、页面 HTTP、Project/Workspace 存储或 session 历史。
