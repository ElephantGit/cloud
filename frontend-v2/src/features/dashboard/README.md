# 仪表盘

仪表盘包含 `/dashboard` 的团队概览和 `/dashboard/sessions` 的真实会话分析。布局及子路由位于 `layout.tsx`、`index.ts`，分析页面、投影、时间轴与请求统计在 `analysis/`，生成客户端在 `api/`。

会话分析始终读取真实后端，不受 `VITE_DEMO_MOCK` 控制。支持来源、项目、Agent、最后一轮状态与时间范围筛选；时间按本地下界含/上界不含转为 UTC，游标绑定用户、租户及筛选。列出自己的普通对话和当前成员可读的手动协作任务 Agent 会话，普通对话不向其他成员共享。详情有概览、模型请求、时间轴、工具统计、完整事件，并可返回来源；Chat 通过 `/chat?conversation=<id>` 选中原对话，失效链接提示已删除或无权访问。

Agent 筛选按会话冻结的完整标识精确匹配，例如 `official/ora-space.opencode`，不是显示名称或执行器短名。输入提示与 B 段预置样例使用同一真实格式，现场会话与预置历史可按同一个 Agent ID 查询。

历史是 PostgreSQL 定型记录快照，不是 Revision 完整归档。未知时间、缺失用量及不完整状态保持原义。历史与模型统计独立读取；授权失败隐藏旧详情，统计刷新失败隐藏旧数值。模型明细注明 fixture/上游来源，汇总覆盖全部已记录请求，趋势仅当前页；没有观测不估算用量。

`task v2:demo:lead` 及合成 `task v2:demo` 沿用现有 B 段装配。陈默账号有明确标为「预置分析样例」的历史，非现场执行且没有伪造请求统计；现场真实 Agent 任务自动进入分析列表。团队 mock 打开时，新分析仍读真实数据。下面的数据来源和计算说明仅针对团队概览。

团队进度与 Agent 产出的概览：本周概览数字、成员（人和 Agent）的进行中/待验收/本周完成及完成率、Agent 最近交付与待验收的结果、每天进入「已完成」的任务数。

## 数据来源

任务聚合接口还不存在，所以页面目前只有演示数据：`src/lib/demo/dashboard.ts` 注册一份符合 `DashboardData` 形状的固定数据集，`page.tsx` 只从这个模块取数，不为展示另算一份数字（成员合计与概览数字一致）。真实统计接入时替换 `src/lib/demo/dashboard.ts`，页面不改。

演示层由 `src/lib/demo/flags.ts` 的 `demoMockEnabled()` 统一开关：只有 `VITE_DEMO_MOCK=1` 才渲染演示仪表盘，未设置时页面保持占位。`npm run check` 和 `npm run e2e` 都不设置这个变量，只用真实数据；页面不直接 import 任何假数据模块，mock 只登记在 `src/lib/demo/` 下作为某条真实后端接口的占位。

任务与小队的数据客户端在 `features/collaboration/` 内部，跨 feature 导入或把它们挪到 `src/lib/` 属于共享改动；在任务聚合写入契约之前，页面不从那里读数据，也不自己拼真实统计。

## 计算

完成率 = （待验收 + 本周完成）/（进行中 + 待验收 + 本周完成），没有任务时为 0；趋势柱高相对本周最忙的一天，全为 0 时按 1 兜底，避免除零。两者都在 `dashboard-model.ts` 里，单独有测试。

## 验证

`dashboard-model.test.ts` 覆盖完成率的边界与趋势缩放，`page.test.tsx` 覆盖开关关闭时的占位和打开时的成员行、完成率、Agent 产出与演示数据说明，`src/lib/demo/dashboard.test.ts` 检查演示数据自洽（概览 = 成员合计、说明里的数字由成员算出、Agent 产出与 Agent 待验收一一对应、趋势合计 = 本周完成）。不需要 v2-server 或数据库。
