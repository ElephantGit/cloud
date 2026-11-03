# 录屏工作流与场景 seed

`demo` 包只提供场景数据，公共演示环境框架负责重置数据库、准备本地 Git 项目、预置用户和启动进程。这里不读取“第一个用户”，不启动服务，也不删除任何数据。

框架准备好每段的用户和项目后，构造 `WorkflowSeed`：`Store` 与 `Pool` 指向同一个演示数据库；`Scope` 是本段用户的成员身份；`ProjectID` 是本段已准备的项目；`AgentCLI` 与 `ModelID` 来自真实执行目录；`Now` 显式指定 seed 时钟。模型不能为 `fixture`。

本地 Git 项目也要使用标准 UUID 作为项目 ID，并设置 `projectrepos.SnapshotRef(project.ID, project.Commit)` 对应的 Git ref。Node 会将 snapshot ref 中的 binding ID 按 UUID 解析；随意起一个文本 ID 会在克隆时被拒绝。seed 会提前检查项目 ID 与 snapshot ref，避免到现场运行才失败。

调用 `SeedNewcomer(ctx, seed)`、`SeedDecomposition(ctx, seed)`、`SeedOps(ctx, seed)`，返回工作流定义与发布快照。三者可以独立调用或任意顺序叠加到一个租户；名称分别为「需求开发」「需求拆分」「每日巡检」。重跑由框架先重置环境，不在这些函数里覆盖已有工作流。

- A 使用 `fixtures/newcomer/` 初始化本地 Git 仓库，不依赖 npm 包或外网 clone。README 中提供可现场粘贴的需求。三个 Agent 在同一次运行的共享工作目录里依次规划、改代码、跑测试。
- B 可以使用自己的项目；「需求拆分」要求结构化输出 `{"tasks":[{"title":"…","description":"…"}]}`。工作流最终输出的 `tasks` 是数组，便于建子任务的消费方直接读取。
- C 使用 `fixtures/ops/` 初始化本地 Git 仓库。「每日巡检」只读检查本地演示服务数据并生成报告，不连接生产系统。seed 已发布基线 `v1`，现场可将草稿发布为 `v2`，再在定时任务中选该版本。每晚 02:00 使用 `Asia/Shanghai`。

C 的三晚历史相对于 `Now` 的上海日期生成，每天恰好一条；记录、运行和节点对话全部持久化，报告标明「预置演示历史」。节点的已保存任务和回答通过运行页原有的历史会话回退视图展示，不伪造实时 Agent 工具事件。未来的现场运行仍经真实工作流执行器执行。

验证：`TEST_DATABASE_URL=... go test -race ./demo` 验证图的发布校验、拆分输出 Schema、三段无顺序依赖、历史链接与用户隔离。现场真实模型验收还需配置 Node 与模型，不能用这些结构测试替代。
