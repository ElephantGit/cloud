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
| `GET/POST /api/v1/tenants/{tid}/workflows` | 契约 | UUID 游标分页、创建草稿；活跃工作流名称在团队内唯一 |
| `GET/PUT/DELETE /api/v1/tenants/{tid}/workflows/{wfid}` | 契约 | 读取、带版本保存、归档；归档后隐藏草稿及版本记录 |
| `POST /api/v1/tenants/{tid}/workflows/{wfid}/publish` | 契约 | 冻结独立快照，并发发布分配不同的版本号 |
| `GET /api/v1/tenants/{tid}/workflows/{wfid}/snapshots` | 契约 | UUID 游标分页读取版本 |
| `GET /api/v1/tenants/{tid}/workflows/{wfid}/snapshots/{snapshotId}` | 契约 | 读取所属团队、工作流下的不可变快照 |

## 工作流资产

草稿保留完整 JSON 对象，包括暂未完成的节点、编辑器注解和未知扩展字段。PUT 中省略的
`description`、`graph` 保持原值；缺少有效版本返回 `428 version_required`，过期版本返回
`409 version_conflict`。同名活跃草稿返回 `409 workflow_conflict`；归档释放名称，不删除快照。
所有工作流路由逐请求检查当前用户及团队的活跃成员关系，数据查询始终限定团队和工作流。
写入请求体上限为 2 MiB；只允许契约定义的顶层字段，拒绝 null、错误类型和多个连续 JSON 文档。

发布目前是**资产快照**：只检查 nodes/edges 数组、唯一节点与连线 ID、一个 start、已知八种
节点类型及有效连线端点。它不证明节点配置、变量作用域、分支或循环语义已经可执行。
本服务尚未提供工作流运行接口；运行器接入时必须补齐发布语义门禁，并重新校验既有快照，
不能因为已有快照就跳过执行准入。前端明确显示执行尚未开放。

保存、归档和发布以工作流行锁串行化；发布只复制结构校验时读取的同一草稿版本，
中途发生编辑则返回版本冲突。草稿版本与发布版本各自递增，发布不改变草稿版本。
状态保存在 PostgreSQL，进程重启后仍可读取；每次发布创建新快照，之后保存不改旧快照。

## 契约之外

暂无。v2 需要而 `api/openapi.json` 没有的接口登记在这里，作为正式 Cloud 要补的契约。

## 与正式 Cloud 的差异

- 登录：没有外部 provider，也没有 attempt 和 PKCE；dev 登录固定为一个演示用户。
- 会话存在同一个库里，只存 token 的 SHA-256；没有 Gateway 现签的内部 JWT。
- 写请求不检查 `Idempotency-Key`，重复发布会产生新快照；工作流编辑/归档检查资源版本，
  没有正式 Cloud 的执行代次隔离。
- 启动时直接建表，没有迁移。
