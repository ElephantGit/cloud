# Agent Run 残留回收：Cloud 责任与证据

[English](residual-reclamation.en.md) | 中文

[有界启动](startup-settlement.md)的 D4/D5 收尾可以先结束业务运行，把尚未查明的资源留在
`agent_run_residuals`。Cloud 的残留回收在原账本上继续取证，证据足够后才取得系统清理责任，沿已有
owned delete 执行删除。它不重新结算 [IssueRun](issue-runs.md)，也不重新开放运行 Workspace。

本文描述 Cloud 第 1 步的实现边界，不作为 Controller、Sandbox Server 的物理终止、部署或联合故障验收证据。
[残留回收 ADR](https://github.com/ora-space/specs/blob/main/decisions/cloud/issue-run/20261018-residual-agent-run-reclamation.md)
仍为 `approved`；本步不是整份 ADR 已实施完成的声明。

## 账本状态和单一责任

| `reclaim_state` | 含义 | 取得责任的字段 |
|---|---|---|
| `reconciling` | 收集原身份事实，尚未授予删除资格 | ID、mode、operation 为空，authorization 为 `{}` |
| `reclaiming` | 已取得一份持久责任，继续推进或等待证据 | 固定 `reclaim_id`、mode 和非空 authorization；`owned_delete` 必须绑定唯一 operation |
| `cleared` | 原责任下的处置已完成 | 保留原责任，同时有 `cleared_at` 和 disposition |

第 1 步的生产授权只使用 `owned_delete`，完成处置只写 `released`。`controlled` 只预留共用内部仲裁位置，
不构成受控管理端点或运维权限。数据库接受的 `terminated_data_retained`、`externally_confirmed` 留给后续受控路径。

自动责任始终绑定同一个 `delete_workspace` operation。重启、换 lease、重试和 quiesce 拒绝不会产生第二个责任或 operation，
也不会把失败的历史 operation 恢复为运行态。普通删除、派发和许可入口继续检查残留保护；内部系统责任不能被当成公开删除开关。
账本、Run、Workspace、Project、operation 的租户、原绑定和 generation 在同一事务里重新比对；UUID 外键本身不证明这些范围一致。

[迁移](../../internal/core/migrations/README.md)只增列。存量 D4/D5 行通过默认值进入 `reconciling`，不推断完成、不回填 D8，
不修改旧 `reason`、`responsibility` 或业务结算。旧行已有非空 `cleared_at` 时明确报错回滚，等待显式兼容处理（A2）。

## 原身份取证与删除资格

`reclaim_targets` 是原计划和事实目标的并集，包括原 operation/effect、预先分配的 sandbox ensure ID、sandbox、Node、
execution、clone、work 和数据目标。迟到事实可以补充原目标，不能将旧目标替换成当前 generation，不能用“查不到”删去未知目标。

新事实追加到 `reclaim_evidence`，包含逻辑事件键、事实、来源和数据库接收时间，不再写入旧 `responsibility.lateFacts`。
相同事实重放复用原回执；新证据、目标补充或状态推进增加 `reclaim_version`。证据接管与授权、许可、完成都使用 Cloud 的既有
串行事务边界，因此完成必须重新看本事务内的完整目标和最新事实。

系统授权要求原身份可定位、admission 已关闭、无冲突、无阻挡中的 Project 操作或其他活动，并满足交付和执行证据条件。
执行侧需要同一 runtime control 身份的关闭输入确认及明确的未完成执行列表，并检查原执行终态；逐目标的终止证据也可证明其
物理执行已封闭。仅在原计划和执行关系确实不存在时才能采用无目标证明，缺行、NULL result 或等待时间均不构成这种证明。

第 1 步只接受原 `deliver_revision` 执行真实登记、核验完成的 Revision 作为交付资格；其他实际交付执行也必须各自核验完成。
当前没有可统一引用的既有“明确放弃交付”记录来源。`failed`、`skipped`、业务超时、没有对象存储或无结果都不自动等于明确放弃（A1），
证据不足的条目继续保持未完成。

获授权后沿已有 quiesce → terminate → cleanup 推进。quiesce 拒绝使原 operation 留在 `retry_wait`，保留原责任及保护，稍后重新取证再试（A3）。
它不恢复新执行资格，不升级为强停，也不重新创建 operation。

每个原计划 sandbox/ensure ID 都需要对应成功终止结果同时包含 `terminated` 和 `lateEnsureFenced`。
仅有 `terminated_at`、ensure 失败、404 或经过一段时间都不够。当前安全目标可以复用已有 owned delete；额外历史 ID 缺少双确认时保持未完成，
不为它改写目标或另发终止 effect（A5）。数据删除 effect 的计划、许可和完成再次检查目标覆盖；同一事务确认数据删除成功、operation 成功、
Workspace 软删除并写 `cleared/released`，不改 Run 的 phase/status/result 或重复追加业务结算活动。

## 既有事实入口与冲突

Cloud 使用现有 `runtime_ack`、创建事实、执行结果和 Revision intake；没有新增 RPC、proto 或 Controller 取证机制。
`runtime_ack` 通过原 control epoch/version、Node、sandbox 和 generation 检查后，将关闭输入和 `unfinishedExecutionIds` 原样接入账本。
会话终态和交付结果按原 execution/work/资源身份接管；Run 已 done 或账本已 cleared 也不能成为拒收一致迟到事实的理由。
成果救援沿既有 Revision 核验路径，不开放 admission，不启动新会话或执行。

一致重放不改原结论。矛盾终态先追加冲突审计并设置 `reclaim_conflicted_at`，提交这些事实及阻挡后才返回稳定的
`409 residual_fact_conflict`（A4）。规范事实、已有 `cleared_at` 和 disposition 保留，后续删除或复用受阻；本步没有解除冲突的入口。

## 对账、恢复与尚未覆盖的范围

`ReconcileAgentRunResidualsOnce` 独立扫描账本的 `reconciling/reclaiming`，不依赖业务 Run 仍处于 `releasing`。
扫描按 `reclaim_checked_at NULLS FIRST, created_at, run_id` 有界轮转；检查时间只是调度记录，不是完成证据。
每个条目重新读取目标、资格和原绑定；已有责任恢复原 operation，继续服从当前 Controller lease 和 effect 许可。
查询失败、证据不足和暂时拒绝均不清账、不扩大授权。

failed create 的原 effects 如果没有现成可主动发现的路径，就继续等待可验证的原身份事实（A6）。Cloud 接收证据和调度责任不证明
Controller 已新增发现能力，更不证明真实容器、迟到 ensure 围栏或物理数据删除已经通过生产验证。

| 后续范围 | 本步边界 |
|---|---|
| 第 2 步：受控运维入口、保留数据、外部确认 | 未实现；内部 `controlled` 仲裁预留不替代认证、意图或命令 |
| 第 3 步：Controller / Sandbox Server 物理契约与逐目标证据 | 本文仅描述 Cloud 的证据要求，不声明物理验收完成 |
| 第 4 步：跨仓联合故障与恢复验收 | 未完成，不以单仓实现或数据库测试代替 |
| D8 历史删除放弃、无账本残留的接管 | 未实现；不补账、不自动重试、不改历史业务结果 |

核心代码入口见 [core 代码地图](../../internal/core/README.md)。测试结果应以对应执行产物为准，本文不声明迁移、并发或端到端测试已经通过。
