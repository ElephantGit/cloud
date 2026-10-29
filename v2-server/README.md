# v2-server

Cloud v2 的**临时**单体后端：一个进程、一个 PostgreSQL 库，实现 `api/openapi.json` 里 v2 用到的子集，供 `frontend-v2` 演示和快速端到端测试。`frontend-v2` 切到正式 Cloud 后删除。

## 运行

需要 Go（版本见 `go.mod`）和 PostgreSQL。仓库根目录的 `compose.yaml` 提供本地 PostgreSQL（127.0.0.1:55432）。v2-server 默认连接其中的 `ora_v2` 库，库不存在时自动创建，启动时建表。

```sh
docker compose -p cloud up -d --wait   # 在仓库根目录
go run .                               # 监听 127.0.0.1:8090
```

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

## 与正式 Cloud 的差异

### 定时触发记录基础

`internal/schedule.Next` 提供服务端时间计算；`Store.RecordOccurrence` 在事务内锁定配置、检查创建者与当前成员关系，并写入 `v2_schedule_occurrences`。尚未安装周期扫描器、自动派发器或 HTTP 触发入口，不调用执行模块。配置页面仍不会触发真实运行。

- 自动触发身份为配置 ID + scheduled + UTC 计划时刻，服务端从时刻生成键；同一时刻不因配置版本变化重复生成。
- 手动触发使用配置 ID + manual + 稳定请求键；同键同版本同请求时刻返回原记录，变更参数返回冲突；显式重跑使用新键。暂停配置允许明确的手动记录，但仍不执行。
- 每次记录冻结配置全文和版本、计划时刻与记录时间。所有状态为 `recorded`，只表示触发已记录，不表示等待、运行或成功。未来真实执行的关联和状态转换需在公共能力接入时补充。
- 同一身份的重试先找已存在记录，因此后续编辑/暂停不会阻断合法重试，也不会改写历史。新自动触发要求版本仍匹配、配置启用、时间已到且符合规则；未来时刻、创建前时刻和非规则时刻被拒绝。
- 配置行锁串行化创建触发与编辑，数据库 UNIQUE 约束兜底防重复。跨租户/用户读写不可用。`Occurrences` 是内部有界分页接口（1–100条），按不可变 UUID 降序，游标为上一页末项 ID，不是时间排序。
- 时间统一为 PostgreSQL 支持的微秒精度；固定间隔沿创建时间锚点，日历规则跳过 DST 空缺，同一本地时刻回拨时仅记录第一次。

这一层接受显式计划时刻，不替调用方选择停机期间哪些时刻应记录。内部状态接口现已实现忙即跳过、同任务占位与基于证据的重启恢复规则，详见 [状态说明](internal/store/occurrence-states.md)；尚未装配真实执行或启动恢复扫描。明确 Busy 且未接收时记为 skipped；接收未知仍按原身份核对。结果终态与资源释放分开，未确认释放前继续阻止重叠。记录防重复不等于外部执行恰好一次，公共 Session/Turn/事件仍由公共执行模块保存。

真实 PostgreSQL 测试覆盖八个并发请求返回同一个触发、不同触发各自存在、编辑后的快照不变、暂停/版本/时刻拒绝、手动重试身份、分页隔离以及关闭数据库连接后重新读取。

- 登录：没有外部 provider，也没有 attempt 和 PKCE；dev 登录固定为一个演示用户。
- 会话存在同一个库里，只存 token 的 SHA-256；没有 Gateway 现签的内部 JWT。
- 写请求不检查 `Idempotency-Key`，没有执行代次隔离；定时任务配置更新单独使用乐观版本检查。
- 启动时直接建表，没有迁移。
