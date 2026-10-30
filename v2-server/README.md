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

## 契约之外

定时任务配置使用独立候选契约 [`schedules.openapi.json`](schedules.openapi.json)，不改变正式 Cloud 的 ScheduledTask（其 issue/agent 执行语义不能表达本次独立对话配置）。前端以独立 Orval 配置生成客户端，后续正式契约收敛时替换这一适配。

| 路由 | 行为 |
|---|---|
| `GET /api/v2/tenants/{tid}/schedule-configs` | 当前创建者在该租户的全部配置；空列表为 `[]` |
| `POST /api/v2/tenants/{tid}/schedule-configs` | 创建配置，返回 201，初始 version 为 1、enabled 为 true |
| `PUT /api/v2/tenants/{tid}/schedule-configs/{id}` | 携带 version 修改配置，保留 enabled 和创建时间 |
| `PATCH /api/v2/tenants/{tid}/schedule-configs/{id}/enabled` | 携带 version 和 enabled 暂停/恢复配置 |

接口要求登录 cookie、有效成员关系及创建者归属。无权限资源返回 404，过期版本返回 409，缺失更新版本返回 428；每次修改递增版本。配置持久存储于 PostgreSQL，服务重新启动后保留。请求大小限制 64 KiB，拒绝未知字段，并校验时间、时区、星期和间隔。

本阶段仅保存无项目的文本对话配置。公共项目、工作流、执行器和模型目录尚未接入，不接受项目关联；执行器/模型文字仅为待接入配置，不能表示可执行绑定。`enabled` 表示保存的计划开关，不代表已部署定时器。没有触发器、真实模型调用或运行历史，`runs` 始终为空；不提供模拟运行、删除或重试入口。真实接入时还需对接公共目录并校验执行选择。

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

### 调度编排与对话消费适配

[`internal/scheduler`](internal/scheduler/README.md) 提供显式注入记录、执行接口和时钟的协调器。Recover 完成持久恢复后才允许 Tick/Manual，首次发现、版本变化和暂停恢复从未来时刻安排；单进程迟到扫描只处理最近到期点，不回补积压。手动请求用固定键找回原时刻和记录，尚未 begin 的记录可继续首次接收，已接收或未知结果仅按原身份核对。

ConversationAdapter 按冻结配置分流：无项目文本对话用公共 StartModel 和明确模型 ID，有项目文本对话用公共 Start 和明确的 projectId、target.executorId。每次触发新建会话，有项目时由公共服务创建独立目录，两路不相互降级。HTTP/前端仍未开放项目绑定，公共项目和执行器目录就绪后才能接入。适配层共用 ReadAttempt/Cancel/End，不直接调用 provider 或 Node，也不另存 Session/Turn。成功要求 AnswerComplete；结果终态和 End 确认分开，End 失败不释放任务占位。缺失绑定、工作流目标明确拒绝，不从演示名称猜测可执行绑定。

此包没有接入 main、HTTP 或前端，也不创建后台 goroutine。生产自动扫描和真实派发仍关闭，公共 Service 实现与记录底座就绪后才能装配。测试通过公共 Stub 注入结果，配合实际 PostgreSQL 验证编排行为；这不是模型调用或生产链路验收。

`server.NewScheduleCoordinator(records, options, now)` 使用页面统一的 `Options.Execution`，不另建执行服务；宿主在公共启动恢复完成后调用 Recover，再驱动 Tick/Manual。Start 与同记录的核对/取消不会并行。启动错误或本地引用未保存时只按原 Caller 读取；无引用且确定没有进行中的 Start，查询未命中可记 skipped/execution_not_started。读取错误或已有引用但缺失记录时保留占位，不重放 prompt。Agent Cancel 结束会话，model-only Cancel 只停当前轮；定时对话两路都随后确认 End，保留历史与目录。详细状态、原因及 released 含义见 [状态说明](internal/store/occurrence-states.md)。

### 定时触发记录基础

`internal/schedule.Next` 提供服务端时间计算；`Store.RecordOccurrence` 在事务内锁定配置、检查创建者与当前成员关系，并写入 `v2_schedule_occurrences`。尚未安装周期扫描器、自动派发器或 HTTP 触发入口，不调用执行模块。配置页面仍不会触发真实运行。

- 自动触发身份为配置 ID + scheduled + UTC 计划时刻，服务端从时刻生成键；同一时刻不因配置版本变化重复生成。
- 手动触发使用配置 ID + manual + 稳定请求键；同键同版本同请求时刻返回原记录，变更参数返回冲突；显式重跑使用新键。暂停配置允许明确的手动记录，但仍不执行。
- 每次记录冻结配置全文和版本、计划时刻与记录时间。初始 `recorded` 只表示触发已记录，不表示已执行；已有同任务占位时直接记为 skipped。执行状态与稳定执行引用由内部证据接口推进，真实公共执行适配尚未装配。
- 同一身份的重试先找已存在记录，因此后续编辑/暂停不会阻断合法重试，也不会改写历史。新自动触发要求版本仍匹配、配置启用、时间已到且符合规则；未来时刻、创建前时刻和非规则时刻被拒绝。
- 配置行锁串行化创建触发与编辑，数据库 UNIQUE 约束兜底防重复。跨租户/用户读写不可用。`Occurrences` 是内部有界分页接口（1–100条），按不可变 UUID 降序，游标为上一页末项 ID，不是时间排序。
- 时间统一为 PostgreSQL 支持的微秒精度；固定间隔沿创建时间锚点，日历规则跳过 DST 空缺，同一本地时刻回拨时仅记录第一次。

这一层接受显式计划时刻，不替调用方选择停机期间哪些时刻应记录。内部状态接口保留同任务占位与重启不补跑，尚未装配真实执行或启动扫描。公共 Busy 仅表示同会话/运行的上一轮仍活动，定时消费保留明确拒绝的 skipped/execution_busy，不做跨任务目录竞争或等待。结果终态与任务占位释放分开，未确认 End 前继续阻止重叠。记录防重复不等于外部执行恰好一次，公共 Session/Turn/事件仍由公共执行模块保存。

真实 PostgreSQL 测试覆盖八个并发请求返回同一个触发、不同触发各自存在、编辑后的快照不变、暂停/版本/时刻拒绝、手动重试身份、分页隔离以及关闭数据库连接后重新读取。

- 登录：没有外部 provider，也没有 attempt 和 PKCE；dev 登录固定为一个演示用户。
- 会话存在同一个库里，只存 token 的 SHA-256；没有 Gateway 现签的内部 JWT。
- 写请求不检查 `Idempotency-Key`，没有执行代次隔离；定时任务配置与小队更新使用乐观版本检查。
- 小队权限把开发登录的 `owner` 角色也当作工作区管理员；成员撤销不会自动关闭其小队编组（由后续接入撤销事务的运行承接）。
- 启动时直接建表，没有迁移。
- 本地 Node 的传输、监督和配套服务位于 [`internal/node`](internal/node/README.md) 与
  [`internal/nodeservices`](internal/nodeservices/README.md)，尚未接入此服务入口。
  配套服务只支持演示仓库的只读 Git、固定 OpenCode 包、fixture 或配置的 OpenAI 兼容 provider；
  本地证书有效 24 小时，关闭后撤销凭据，无正式 Cloud 的身份、Git 交付或证书轮换机制。
- [`internal/nodehistory`](internal/nodehistory/README.md) 将固定 Node 的历史转换为替换快照，
  只读其本地 JSONL 恢复完整结果；历史缺失、损坏或超限明确失败。提供真实 Node + PostgreSQL
  的超长回答验证，但尚未接入页面 Session/Turn 存储、公共执行服务或 HTTP 路由。
