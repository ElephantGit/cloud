# v2-server

Cloud v2 的**临时**单体后端：一个进程、一个 PostgreSQL 库，实现 `api/openapi.json` 里 v2 用到的子集，供 `frontend-v2` 演示和快速端到端测试。`frontend-v2` 切到正式 Cloud 后删除。

## 运行

需要 Go（版本见 `go.mod`）和 PostgreSQL。仓库根目录的 `compose.yaml` 提供本地 PostgreSQL（127.0.0.1:55432）。v2-server 默认连接其中的 `ora_v2` 库，库不存在时自动创建，启动时建表。

```sh
docker compose -p cloud up -d --wait   # 在仓库根目录
go run .                               # 监听 127.0.0.1:8090
```

使用本地 Node 配套服务时，自签证书从服务创建起有效 24 小时，不会自动轮换。连续运行超过一天需重启后端以重新生成证书；长时间演示应在证书到期前安排重启。

| 环境变量 | 默认值 | 说明 |
|---|---|---|
| `V2_ADDR` | `127.0.0.1:8090` | 监听地址 |
| `V2_DATABASE_URL` | `postgres://ora:ora-local@127.0.0.1:55432/ora_v2?sslmode=disable` | PostgreSQL |

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
| `GET /api/v1/tenants/{tid}/members` | 契约 | 当前用户所在工作区成员，一页返回全部，按显示名排序；非成员 `403 forbidden`，非法租户 id `404 not_found` |
| `GET /api/v1/tenants/{tid}/spaces` | 契约 | 当前用户所在工作区的空间，一页返回全部；v2 每个租户一个「演示空间」，`role` 取调用者的工作区角色；非成员 `403 forbidden` |
| `GET /api/v1/tenants/{tid}/spaces/{sid}/plugins` | 契约 | 该空间里 `kind=agent` 且已安装的插件，一页返回全部；空间不属于调用者的工作区时 `403 forbidden`，非法 id `404 not_found` |

## 契约之外

定时任务配置使用独立候选契约 [`schedules.openapi.json`](schedules.openapi.json)，不改变正式 Cloud 的 ScheduledTask（其 issue/agent 执行语义不能表达本次独立对话配置）。前端以独立 Orval 配置生成客户端，后续正式契约收敛时替换这一适配。

| 路由 | 行为 |
|---|---|
| `GET /api/v2/tenants/{tid}/schedule-configs` | 当前创建者在该租户的全部配置；空列表为 `[]` |
| `POST /api/v2/tenants/{tid}/schedule-configs` | 创建配置，返回 201，初始 version 为 1、enabled 为 true |
| `PUT /api/v2/tenants/{tid}/schedule-configs/{id}` | 携带 version 修改配置，保留 enabled 和创建时间 |
| `PATCH /api/v2/tenants/{tid}/schedule-configs/{id}/enabled` | 携带 version 和 enabled 暂停/恢复配置 |
| `GET /api/v2/tenants/{tid}/schedule-resources` | 当前有效成员的真实项目、模型和执行器投影；分别标明目录是否可用，不返回后端路径或凭据 |

接口要求登录 cookie、有效成员关系及创建者归属。无权限资源返回 404，过期版本返回 409，缺失更新版本返回 428；每次修改递增版本。配置持久存储于 PostgreSQL，服务重新启动后保留。请求大小限制 64 KiB，拒绝未知字段，并校验时间、时区、星期和间隔。

对话配置保存时按当前权限验证绑定：无项目的 model ID 必须来自共享 Catalog；有项目的 projectId 必须来自当前租户的共享项目存储，target.executorId 必须匹配共享执行器和所选 model ID。agent 字段只用于展示，不作为执行身份。公共目录未就绪或选择失效时拒绝写入，不猜测默认模型/项目。兼容未绑定执行的文本配置：无项目、空模型、无 executorId 可先保存，运行时仍不能通过 model_required 门禁。

资源接口复用 `Options.Catalog` 和共享 `ListProjects`/`GetProject`，没有另一份项目/模型目录。响应的 available 区分目录不可用与可用但为空，先校验当前用户/租户/成员，再返回数据；只投影名称和稳定ID，不公开镜像目录、provider binding或凭据。项目/模型/执行器在每次保存时重检，真正执行时仍由公共服务再次鉴权。

配置响应中的 `runs` 保留兼容空列表。前端另消费执行操作和历史领域的独立候选契约 `schedule-actions.openapi.json` / `schedule-history.openapi.json`；这两份契约在本线是提供方快照，不代表相应路由已装配。公开执行 Service、调度 Host 和历史后端就绪后再联调，缺少时页面明确显示不可用，不生成模拟结果。

专用 HTTP 集成测试按此候选 OpenAPI 验证响应，并覆盖创建编辑、暂停恢复、服务替换、归属隔离及错误输入；数据库测试覆盖连接池关闭重开和并发版本冲突。

### 定时任务职责与旧引用兼容

定时任务只负责计划、触发身份、防重叠、调用提供方的执行接口，以及关联和展示外部运行事实。工作流的定义、发布快照、参数模式/语义校验、节点调度和执行由工作流模块拥有；任务目录、描述、项目和执行绑定由任务模块拥有。定时模块不要求这些提供方实现自己预设的解析协议。

当前写入契约只接受已接通的 chat 配置。workflow/task 写入明确返回 `503 schedule_target_unavailable`；没有 ScheduleTargetResolver、参数字符串化或可由替身开启的引用写入通道。后续接入以提供方实际交付的接口为准。

之前已保存的引用仍按原 JSON 读取，可暂停/恢复计划；只读候选类型 `ScheduleLegacyTarget` 保留未知字段，不解释、改写或校验其中的提供方参数。没有删除数据库数据或迁移其字段，前端也不提供引用参数编辑/执行入口。

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

成员是显式类型化引用：人类引用 user 身份，Agent 引用 Space Agent 身份，不使用裸 memberId。人类必须是当前有效的租户成员；Agent 必须是同工作区活动且观测为已安装的 Space Agent。当前工作区成员可读取小队与成员；编辑、增员与归档只限工作区管理员或本队有效人类维护者，Agent 不能成为维护者。写操作按小队 version 做乐观校验：过期返回 409 `version_conflict`，缺 version 返回 428，归档后返回 409 `squad_archived`。归档结束全部有效编组、保留历史且不提供恢复；移除现任 leader 会在同一事务清除 leader 配置。

Agent 成员引用 `v2_space_agents` 里观测为已安装的插件，候选由契约的空间插件读取路由提供（见上方契约路由表）；没有安装、卸载写接口，也不使用虚构 Agent。

小队与成员关系不授予私人模型、Git 凭据或运行时权限，也不触发 Agent 执行、leader 路由或自动委派。

### 项目仓库

项目使用独立同步候选契约 [`projects.openapi.json`](internal/projectrepos/projects.openapi.json)：
`GET/POST /api/v2/tenants/{tid}/projects` 与 `GET /api/v2/tenants/{tid}/projects/{pid}`。
创建只接受匿名 HTTPS 仓库 URL 与分支，完成一次 mirror clone、验证分支并固定 snapshot 后才写入
已有的 PostgreSQL 项目记录；clone 或分支检查失败不创建项目。之后不 fetch，也不向上游 commit/push。
列表按 ID 升序分页，读取要求当前租户成员关系；创建者来自登录身份。响应不暴露 mirror 路径、凭据或 snapshot ref。

[`projectrepos.New`](internal/projectrepos/README.md) 通过 `server.Options.Projects` 注入。
`main` 尚未装配该依赖，当前启动的项目接口返回 503 `projects_unavailable`。
内部 `SeedDemo` 可创建或复用带真实 Git mirror 的演示项目，但构造服务和 HTTP 请求不会自动调用它。
本接口不伪造正式 v1 创建契约中的 Space、Workspace 或异步 Operation，也不创建执行目录。

## 进程内 Agent 执行接口

[`internal/execution`](internal/execution/README.md) 的公共 Go 接口由
`server.Options.Execution`（`execution.Service`）与 `server.Options.Catalog`（`execution.Catalog`）
统一注入，页面 handler 通过保存的 Options 使用同一个执行后端。页面服务测试可注入显式 callback Stub，
不必各自创建执行实现或修改启动入口。

`server.New` 将未配置的 Execution/Catalog 字段默认设为同一个未配置 Stub；显式传入的依赖保留不变。
这两个字段分别配置：即使 Execution 同时实现 Catalog，省略 Catalog 仍表示该清单不可用。

`main` 目前给这两个字段传入同一个未配置的 `executiontest.Stub`，所有执行、读取与目录调用均返回
`execution.ErrUnavailable`；它不会伪造运行、清单或持久历史。替换为真实执行服务只改变启动装配，
页面继续依赖同一接口。本注入点不新增 HTTP 契约。

[`internal/modelturn`](internal/modelturn/README.md) 提供无项目的纯文本单轮执行器，
通过配置的 provider 解析 JSON / SSE，返回完整答案、停止原因和实际可得的 usage。
取消会停止上游调用；截断、损坏或缺少终态的响应明确失败。该组件尚未装配到公共执行服务，
admission、会话和事件持久化、重启恢复仍由调用方负责。

## 与正式 Cloud 的差异

- 登录：没有外部 provider，也没有 attempt 和 PKCE；dev 登录固定为一个演示用户。
- 会话存在同一个库里，只存 token 的 SHA-256；没有 Gateway 现签的内部 JWT。
- 写请求不检查 `Idempotency-Key`，没有执行代次隔离；定时任务配置与小队更新使用乐观版本检查。
- 小队权限把开发登录的 `owner` 角色也当作工作区管理员；成员撤销不会自动关闭其小队编组（由后续接入撤销事务的运行承接）。
- 空间与插件：每个租户固定一个「演示空间」，开发登录时自动创建；里面只登记一个真实安装的 Agent 插件——`official/ora-space.opencode@0.6.11`，即 [`scripts/prepare-node.sh`](scripts/prepare-node.sh) 校验 SHA 后准备、chat、工作流与定时任务真正加载的那个。没有安装、卸载写入接口，`kind` 恒为 `agent`，安装计数与请求者不记录；`SpaceAgentPlugins` 的读法在 `#774` 的 `Catalog.ListExecutors` 装配好后可以换成真实来源，契约形状不变。
- 成员列表：`memberships` 没有成员 id、版本和加入来源，所以 `id` 直接用 user id、`version` 恒为 1、`status` 取用户状态，`joinMethod` / `joinedAt` / `actorDisplayName` 恒为空；分页参数被忽略，一页返回全部。切到正式 Cloud 的成员接口时替换。
- 启动时直接建表，没有迁移。
- 本地 Node 的传输、监督和配套服务位于 [`internal/node`](internal/node/README.md) 与
  [`internal/nodeservices`](internal/nodeservices/README.md)，尚未接入此服务入口。
  配套服务只支持演示仓库的只读 Git、固定 OpenCode 包、fixture 或配置的 OpenAI 兼容 provider；
  本地证书有效 24 小时，关闭后撤销凭据，无正式 Cloud 的身份、Git 交付或证书轮换机制。
- [`internal/nodehistory`](internal/nodehistory/README.md) 将固定 Node 的历史转换为替换快照，
  只读其本地 JSONL 恢复完整结果；历史缺失、损坏或超限明确失败。提供真实 Node + PostgreSQL
  的超长回答验证，但尚未接入页面 Session/Turn 存储、公共执行服务或 HTTP 路由。
