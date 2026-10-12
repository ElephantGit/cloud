# gitserver：沙盒 Git 访问

[中文](README.md) | [English](README.en.md)

只以 smart HTTP 向沙盒提供 Project 仓库（`/{tenantId}/{projectId}.git`），见 `proposed` 的 ADR
`cloud/git-server/0`。每项权限都由 `internal/core` 与 PostgreSQL 判定，本包不保存授权状态。

| 文件 | 职责 |
|---|---|
| `config.go` | 部署引用与监听 TLS：仓库监听要求沙盒 CA 签发的客户端证书；ref 监听固定 Cloud 的叶证书 |
| `scope.go` | `RuntimeScope`：只接受 `git-access` 用途的 SPIFFE 身份，同一 CA 签发的 WSS、model-access 证书都不接受 |
| `repository.go` | smart HTTP 的四个端点、规范 ID、首次获授权写入时建仓（先暂存再 rename）、按精确名称查交付 ref |
| `service.go` | 带 `Ora-Execution-Id` 的逐请求授权、经 CGI 调用 `git http-backend`、ref 查询与健康检查 |
| `hook.go` | 一次性推送令牌、私有 Unix socket hook 通道与 `RunPreReceive` hook 主体 |

每个仓库请求从证书得到认证目标，再请 Core 判定所声明的执行能否读（恢复前序 Revision 的会话）或写（交付）。
任何拒绝都是同一个 `403`，仓库不存在也一样；数据库故障返回 `503`，绝不放行。交付只能新建
`refs/ora/deliveries/<workId>`：`pre-receive` 在写入任何 ref 之前重新做一次完整的 Core 判定，除非每个更新都是
用提交新建这个 ref，否则整次推送拒绝。同一尝试的重放推送显示 up to date，不会失败。

仓库策略与 hook 路径通过 `GIT_CONFIG_*` 环境变量作用于每个 git 进程，而不写进仓库，因此重启（hook 目录变化）
或旧仓库都无法削弱它们。对象传输与 hook 都不持有数据库事务。

测试用真实 git 客户端、`git http-backend` 与 TLS，配合策略替身覆盖边界矩阵；Core 的 PostgreSQL 测试覆盖判定本身，
`integration/git_server_test.go` 用真实 Cloud 状态下的交付、以发布的命令作为 hook 跑通全链路。长时间的 fetch
不在传输中途重新授权；写入在 pre-receive 中重新授权。
