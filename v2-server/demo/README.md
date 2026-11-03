# 录屏工作流与场景 seed

## 演示环境框架

`task v2:demo:newcomer|lead|ops` 和合成的 `task v2:demo` 由 `scripts/demo.sh` 执行，用法见 [`../README.md`](../README.md#录屏演示环境)。`cmd/v2-demo` 提供它调用的三个子命令：`env` 输出本段的环境变量，`reset` 清空数据目录、重建数据库并写入合并后的用户文件，`seed` 写入本段数据。

- `env.go`：各段的库、数据目录和端口表；重置；`Seeder` 的写入辅助：`Scope` 按邮箱找到本段账号，`Project` 把 `fixtures/<段>/` 准备成一个本地 Git 提交的项目（UUID 与 snapshot ref 齐全），`WorkflowSeed` 把账号和项目交给下面的工作流 seed。
- `scenes.go`：三段的接线和合成时的顺序。
- `newcomer.go`、`lead.go`、`ops.go`：各段的数据，由各段负责人维护。每段只用 `users/<段>.json` 里的账号和自己的项目，不依赖其他段先跑过。
- B 段（`lead.go`）：陈默（owner）带苏芮、周衡和 OpenCode、Claude Code 组成「看板小队」，项目「协作看板服务」，「需求拆分」工作流，以及几张人类成员已经在做或做完的任务。Agent 任务在录屏时现场执行，seed 不写任务运行和评论。

## 工作流 seed

`demo` 包只提供场景数据，公共演示环境框架负责重置数据库、准备本地 Git 项目、预置用户和启动进程。这里不读取“第一个用户”，不启动服务，也不删除任何数据。

框架准备好每段的用户和项目后，构造 `WorkflowSeed`：`Store` 与 `Pool` 指向同一个演示数据库；`Scope` 是本段用户的成员身份；`ProjectID` 是本段已准备的项目；`AgentCLI` 与 `ModelID` 来自真实执行目录；`Now` 显式指定 seed 时钟。模型不能为 `fixture`。

本地 Git 项目也要使用标准 UUID 作为项目 ID，并设置 `projectrepos.SnapshotRef(project.ID, project.Commit)` 对应的 Git ref。Node 会将 snapshot ref 中的 binding ID 按 UUID 解析；随意起一个文本 ID 会在克隆时被拒绝。seed 会提前检查项目 ID 与 snapshot ref，避免到现场运行才失败。

调用 `SeedNewcomer(ctx, seed)`、`SeedDecomposition(ctx, seed)`、`SeedOps(ctx, seed)`，返回工作流定义与发布快照。三者可以独立调用或任意顺序叠加到一个租户；名称分别为「需求开发」「需求拆分」「每日巡检」。重跑由框架先重置环境，不在这些函数里覆盖已有工作流。

- A 使用 `fixtures/newcomer/` 初始化本地 Git 仓库，不依赖 npm 包或外网 clone。README 中提供可现场粘贴的需求。三个 Agent 在同一次运行的共享工作目录里依次规划、改代码、跑测试。
- B 可以使用自己的项目；「需求拆分」要求结构化输出 `{"tasks":[{"title":"…","description":"…"}]}`。工作流最终输出的 `tasks` 是数组，便于建子任务的消费方直接读取。
- C 使用 `fixtures/ops/` 初始化本地 Git 仓库。「每日巡检」只读检查本地演示服务数据并生成报告，不连接生产系统。seed 已发布基线 `v1`，现场可将草稿发布为 `v2`，再在定时任务中选该版本。每晚 02:00 使用 `Asia/Shanghai`。

C 的三晚历史相对于 `Now` 的上海日期生成，每天恰好一条；记录、运行和节点对话全部持久化，报告标明「预置演示历史」。节点的已保存任务和回答通过运行页原有的历史会话回退视图展示，不伪造实时 Agent 工具事件。未来的现场运行仍经真实工作流执行器执行。

验证：`TEST_DATABASE_URL=... go test -race ./demo` 验证图的发布校验、拆分输出 Schema、三段无顺序依赖、历史链接与用户隔离。现场真实模型验收还需配置 Node 与模型，不能用这些结构测试替代。
