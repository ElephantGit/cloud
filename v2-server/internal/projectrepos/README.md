# 项目仓库文件夹

`projectrepos.New(store, Options{Root: absolutePath})` 创建仓库服务，交给 `server.Options.Projects`。
Root 是后端配置，不接浏览器目录。`Create(ctx, scope, Input)` 检查成员关系后同步执行一次
`git clone --mirror`，验证指定分支，创建固定 snapshot ref，最后写入 Project。
`SnapshotRef(projectID, commit)` 与固定 Node 的 clone 协议匹配；M4 将 Project.MirrorPath 交给
nodeservices.Repository，并使用 Project.SnapshotRef 与 Commit 创建对话目录。

内部 `SeedDemo(ctx, scope)` 为一条命令的 fixture 验收准备本地仓库：在 Root 下生成私有临时源，
git init、提交 README、clone --mirror，按同一流程固定 snapshot 并保存 `fixture://demo` 项目。
同租户重复调用复用已有项目，不接受调用者指定的源路径；HTTP 仍禁止 fixture/file scheme。

默认只接受无 userinfo、query、fragment 的 HTTPS 仓库地址，不接受 SSH、本地路径或凭据引用。
Git 不继承用户配置、HOME、代理、SSH、askpass 或凭据 helper；不执行 hooks，不跟随 HTTP 重定向。
命令有两分钟期限、有限输出、不经 shell；错误不返回 Git stderr。测试可显式启用 AllowLoopbackHTTP，
仅允许数字 loopback HTTP 地址，生产不能启用。

创建后删除 mirror 的 origin 配置，后续不 fetch。上游变化不会改变 Project.Commit 或 snapshot ref。
clone/分支失败不写项目并清理准备目录。数据库返回错误后保留已准备的镜像，避免连接中断时删除可能已被记录引用的目录；
这类孤立镜像由显式清理命令处理。普通 End 和重启不删除仓库或会话目录，Agent 改动不写回这里。

## 契约之外

临时接口在本目录 `projects.openapi.json` 定义，并由真实 Git、PostgreSQL 与 HTTP 测试校验：

- `POST /api/v2/tenants/{tid}/projects`：name、repositoryUrl、defaultBranch；同步成功返回 201 和 Project。
- `GET /api/v2/tenants/{tid}/projects`：按 ID 升序分页，limit 为 1–100，after 为前页最后一个 ID。
- `GET /api/v2/tenants/{tid}/projects/{pid}`：按当前成员归属读取项目。

公开 Project 只有 id、tenantId、ownerUserId、name、repositoryUrl、defaultBranch、commitId、createdAt。
不公开镜像本地路径、snapshot ref 或凭据。创建者由登录身份记录，同租户成员读取时不会变成读取者。

正式 `api/openapi.json` 的 v1 创建接口返回 202 + Project/Operation/main Workspace，并要求 Project.spaceId。
临时后端只有同步创建的仓库记录，且执行目录在 Start 时才建，因此不能伪造 Workspace、Space 或 Operation 来凑正式形状。
本模块不注册该 v1 创建、列表或读取接口；前端需要显式使用以上候选接口。没有修改正式 OpenAPI。
