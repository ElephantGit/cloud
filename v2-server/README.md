# v2-server

Cloud v2 的**临时**单体后端：一个进程、一个 PostgreSQL 库，实现 `api/openapi.json` 里 v2 用到的子集，供 `frontend-v2` 演示和快速端到端测试。`frontend-v2` 切到正式 Cloud 后删除。

## 运行

需要 Linux/WSL、Git、Go（版本见 `go.mod`）和 PostgreSQL。仓库根目录的 `compose.yaml` 提供本地 PostgreSQL（127.0.0.1:55432）。v2-server 默认连接其中的 `ora_v2` 库，库不存在时自动创建，启动时建表。Node 数据目录应放在 Linux 原生文件系统。

```sh
docker compose -p cloud up -d --wait   # 在仓库根目录
go run .                               # 监听 127.0.0.1:8090
```

使用本地 Node 配套服务时，自签证书从服务创建起有效 24 小时，不会自动轮换。连续运行超过一天需重启后端以重新生成证书；长时间演示应在证书到期前安排重启。

| 环境变量 | 默认值 | 说明 |
|---|---|---|
| `V2_ADDR` | `127.0.0.1:8090` | 监听地址 |
| `V2_DATABASE_URL` | `postgres://ora:ora-local@127.0.0.1:55432/ora_v2?sslmode=disable` | PostgreSQL |
| `V2_DATA_DIR` | `$HOME/.ora-v2` | 仓库 mirror、Node 与对话目录；同一目录只允许一个进程持有，使用短路径以满足 Unix socket 长度限制 |
| `V2_NODE_ASSETS_DIR` | 未设置 | `task v2:node:prepare` 生成的固定 Node、host、guardian、Deno 和 OpenCode 资源；未设置时仅启用纯模型路径 |
| `V2_MODEL_BASE_URL` / `V2_MODEL_API_KEY` / `V2_MODEL_NAME` | 未设置 | 可选的一组 OpenAI 兼容 provider；必须同时设置，Agent 与纯模型共用，清单只公开模型名称与版本 |

测试需要 `TEST_DATABASE_URL`（任意可建 schema 的库），每个测试用独立的 schema：

```sh
TEST_DATABASE_URL='postgres://ora:ora-local@127.0.0.1:55432/ora?sslmode=disable' go test -race ./...
```

## 已实现

| 路由 | 来源 | 说明 |
|---|---|---|
| `GET /healthz` | 契约 | 数据库可达即 `ok` |
| `GET /auth/providers` | Gateway | 只有 `dev` |
| `POST /auth/login` | Gateway | `dev` 登录：直接以演示用户登录、加入"演示团队"，设置 `ora_session` Cookie，`authorizationUrl` 即 `returnTo`（只接受站内路径） |
| `POST /auth/logout` | Gateway | 吊销会话，`204` |
| `GET /api/v1/me` | 契约 | 当前用户；无会话 `401 unauthenticated` |
| `GET /api/v1/me/tenants` | 契约 | 当前用户所在租户，一页返回全部，`nextCursor` 为空 |

## 契约之外

定时任务配置使用独立候选契约 [`schedules.openapi.json`](schedules.openapi.json)，不改变正式 Cloud 的 ScheduledTask（其 issue/agent 执行语义不能表达本次独立对话配置）。前端以独立 Orval 配置生成客户端，后续正式契约收敛时替换这一适配。

| 路由 | 行为 |
|---|---|
| `GET /api/v2/tenants/{tid}/schedule-configs` | 当前创建者在该租户的全部配置；空列表为 `[]` |
| `POST /api/v2/tenants/{tid}/schedule-configs` | 创建配置，返回 201，初始 version 为 1、enabled 为 true |
| `PUT /api/v2/tenants/{tid}/schedule-configs/{id}` | 携带 version 修改配置，保留 enabled 和创建时间 |
| `PATCH /api/v2/tenants/{tid}/schedule-configs/{id}/enabled` | 携带 version 和 enabled 暂停/恢复配置 |

接口要求登录 cookie、有效成员关系及创建者归属。无权限资源返回 404，过期版本返回 409，缺失更新版本返回 428；每次修改递增版本。配置持久存储于 PostgreSQL，服务重新启动后保留。请求大小限制 64 KiB，拒绝未知字段，并校验时间、时区、星期和间隔。

定时任务配置目前仅保存无项目的文本对话配置，尚未对接下述公共执行服务；不接受项目关联，执行器/模型文字不能表示已校验的可执行绑定。`enabled` 表示保存的计划开关，不代表已部署定时器。没有定时触发器或运行历史，`runs` 始终为空；不提供模拟运行、删除或重试入口。

专用 HTTP 集成测试按此候选 OpenAPI 验证响应，并覆盖创建编辑、暂停恢复、服务替换、归属隔离及错误输入；数据库测试覆盖连接池关闭重开和并发版本冲突。

小队协作使用独立候选契约 [`squads.openapi.json`](squads.openapi.json)。正式 Cloud 尚无小队接口（P1 真实编组由 circus#750 承接），本批不修改 `api/openapi.json`；#750 合入后按正式契约收口。

| 路由 | 行为 |
|---|---|
| `GET /api/v2/tenants/{tid}/squads` | 列出未归档小队及实时人类/Agent/维护者计数 |
| `POST /api/v2/tenants/{tid}/squads` | 创建小队，并在同一事务加入创建者为人类维护者 |
| `GET /api/v2/tenants/{tid}/squads/{sid}` | 小队详情，包含已结束的编组历史 |
| `PUT /api/v2/tenants/{tid}/squads/{sid}` | 编辑名称、简介、协作说明与 leader，携带 version |
| `POST /api/v2/tenants/{tid}/squads/{sid}/archive` | 归档，结束全部有效编组并清除 leader |
| `POST /api/v2/tenants/{tid}/squads/{sid}/members` | 加入人类或 Agent 成员，携带 version |
| `PUT /api/v2/tenants/{tid}/squads/{sid}/members/{mid}` | 修改职责文本与维护者标记，携带 version |
| `DELETE /api/v2/tenants/{tid}/squads/{sid}/members/{mid}?version=N` | 结束编组并保留历史 |

成员是显式类型化引用：人类引用 user 身份，Agent 引用 Space Agent 身份，不使用裸 memberId。人类必须是当前有效的租户成员；Agent 必须是同工作区的活动 Space Agent。当前工作区成员可读取小队与成员；编辑、增员与归档只限工作区管理员或本队有效人类维护者，Agent 不能成为维护者。写操作按小队 version 做乐观校验：过期返回 409 `version_conflict`，缺 version 返回 428，归档后返回 409 `squad_archived`。归档结束全部有效编组、保留历史且不提供恢复；移除现任 leader 会在同一事务清除 leader 配置。

v2-server 还没有 Space Agent 目录接口：`v2_space_agents` 只是 Agent 引用的校验接缝，没有 HTTP 读写；Agent 目录仍应由共享的 Space Agent 读取提供，本批不另造目录 API，也不使用虚构 Agent。

小队与成员关系不授予私人模型、Git 凭据或运行时权限，也不触发 Agent 执行、leader 路由或自动委派。

## 项目与执行目录

项目使用 [`projects.openapi.json`](internal/projectrepos/projects.openapi.json) 的同步候选契约：
`GET/POST /api/v2/tenants/{tid}/projects` 与 `GET /api/v2/tenants/{tid}/projects/{pid}`。
创建时只接受匿名 HTTPS 仓库 URL 与分支，完成一次 mirror clone 并固定 snapshot 后才记录项目；
clone 或分支检查失败不产生项目。后续不 fetch，也不向上游 commit/push。此接口不伪造正式契约中
尚未实现的 Space、Operation 或 Workspace。

普通启动会为开发登录的演示团队创建或复用一个真实的 `fixture://demo` 项目，带本地 Git mirror 与固定
snapshot；项目清单从同一 PostgreSQL 记录读取，重启不重复创建。该可信 seed 不从浏览器接收路径，
HTTP 创建接口仍不接受 `fixture://` 或本地仓库。`-fixture-check` 使用同一项目。

[`catalog.openapi.json`](internal/server/catalog.openapi.json) 提供
`GET /api/v2/tenants/{tid}/execution/models` 和 `GET /api/v2/tenants/{tid}/execution/executors`。
两者均检查登录与租户成员关系，响应为 `{ "items": [...] }`。始终提供免密钥的 `fixture` 模型；
有 Node 资源时提供 `opencode:fixture` 执行器。有完整 provider 环境配置时再提供 `configured` 模型和
`opencode:configured` 执行器。无 Node 资源时执行器列表为空。

## 公共执行服务

启动入口装配 [`internal/execution`](internal/execution/README.md) 的 `Service` 与 `Catalog`，通过
`server.Options.Execution/Catalog` 供页面领域服务调用。Session、Turn、连续事件、结果和 Caller attempt
均保存在 PostgreSQL；公共服务的执行、取消、End 和读取不依赖观察者连接的生命周期。页面对话、工作流、
定时触发 HTTP 与 UI 由相应页面服务接入，此处不新增这些页面的执行路由。

有项目的调用使用一个项目 Node、每个独立对话/定时触发的新 checkout；同一 workflow run 的节点串行共用
checkout。无项目的 `StartModel` 直接调用 [`internal/modelturn`](internal/modelturn/README.md)，不创建目录。
同一 caller attempt 返回原 Ref；同一对话或 workflow run 尚有未结束的一轮时返回 Busy。

有项目 Agent 的活动轮被 Cancel 后，固定 Node 结束整个会话，停止确认后返回真实终态，后续新 Append
返回 `ErrSessionEnded`。纯模型 Cancel 只停止当前轮，之后可继续。对已经终态的旧 Ref 重试 Cancel 只返回
原结果，不影响后续轮次。End 停止并结束会话，历史仍可读；重新启动时清理旧 Node、将未完成轮记为
interrupted，并将旧会话全部结束，不尝试恢复 Agent 或重放上下文。

fixture 仅替代模型，上下游仍使用真实 Git、Node、OpenCode 和 shell 工具：

```sh
# 在仓库根目录准备资源，然后在 v2-server 中运行；资源与数据路径放在 Linux 原生磁盘。
task v2:node:prepare
V2_NODE_ASSETS_DIR=/absolute/path/to/assets go run . -fixture-check
# 使用真实 PostgreSQL 的公共 Service 验证（包括磁盘工具结果与重启读取）
V2_NODE_ASSETS_DIR=/absolute/path/to/assets TEST_DATABASE_URL='postgres://ora:ora-local@127.0.0.1:55432/ora?sslmode=disable' go test -race -count=1 -run TestReal -v ./internal/executionruntime
```

目录不会在 End 或重启时自动删除。停止服务后运行 `go run . -cleanup`，显式清理本数据根登记的 Node
与对话目录；保留项目 mirror 和数据库历史。此命令拒绝与持有同一数据目录的服务并行执行。
`server.New` 将未配置的 Execution/Catalog 字段默认设为同一个未配置 Stub；显式传入的依赖保留不变。
这两个字段分别配置：即使 Execution 同时实现 Catalog，省略 Catalog 仍表示该清单不可用。

## 与正式 Cloud 的差异

- 登录：没有外部 provider，也没有 attempt 和 PKCE；dev 登录固定为一个演示用户。
- 会话存在同一个库里，只存 token 的 SHA-256；没有 Gateway 现签的内部 JWT。
- HTTP 写请求不检查 `Idempotency-Key`；公共执行调用通过 Caller attempt 去重，定时任务配置与小队更新使用乐观版本检查。
- 小队权限把开发登录的 `owner` 角色也当作工作区管理员；成员撤销不会自动关闭其小队编组。
- 启动时直接建表，没有迁移。
- 本地 Node 的传输、监督和配套服务位于 [`internal/node`](internal/node/README.md) 与
  [`internal/nodeservices`](internal/nodeservices/README.md)，通过项目 Agent driver 装配到服务入口。
  配套服务只支持演示仓库的只读 Git、固定 OpenCode 包、fixture 或配置的 OpenAI 兼容 provider；
  本地证书有效 24 小时，关闭后撤销凭据，无正式 Cloud 的身份、Git 交付或证书轮换机制。
- [`internal/nodehistory`](internal/nodehistory/README.md) 将固定 Node 的历史转换为替换快照，
  只读其本地 JSONL 恢复完整结果；历史缺失、损坏或超限明确失败。提供真实 Node + PostgreSQL
  的超长回答验证；公共执行服务在 PostgreSQL 提交替换快照与终态后才 ACK Node。
