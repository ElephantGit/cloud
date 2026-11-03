# 仪表盘

团队进度与 Agent 产出的概览：本周概览数字、成员（人和 Agent）的进行中/待验收/本周完成及完成率、Agent 最近交付与待验收的结果、每天进入「已完成」的任务数。

## 数据来源

任务聚合接口还不存在，所以页面目前只有演示数据：`src/lib/demo/dashboard.ts` 注册一份符合 `DashboardData` 形状的固定数据集，`page.tsx` 只从这个模块取数，不为展示另算一份数字（成员合计与概览数字一致）。真实统计接入时替换 `src/lib/demo/dashboard.ts`，页面不改。

演示层由 `src/lib/demo/flags.ts` 的 `demoMockEnabled()` 统一开关：只有 `VITE_DEMO_MOCK=1` 才渲染演示仪表盘，未设置时页面保持占位。`npm run check` 和 `npm run e2e` 都不设置这个变量，只用真实数据；页面不直接 import 任何假数据模块，mock 只登记在 `src/lib/demo/` 下作为某条真实后端接口的占位。

任务与小队的数据客户端在 `features/collaboration/` 内部，跨 feature 导入或把它们挪到 `src/lib/` 属于共享改动；在任务聚合写入契约之前，页面不从那里读数据，也不自己拼真实统计。

## 计算

完成率 = （待验收 + 本周完成）/（进行中 + 待验收 + 本周完成），没有任务时为 0；趋势柱高相对本周最忙的一天，全为 0 时按 1 兜底，避免除零。两者都在 `dashboard-model.ts` 里，单独有测试。

## 验证

`dashboard-model.test.ts` 覆盖完成率的边界与趋势缩放，`page.test.tsx` 覆盖开关关闭时的占位和打开时的成员行、完成率、Agent 产出与演示数据说明，`src/lib/demo/dashboard.test.ts` 检查演示数据自洽（概览 = 成员合计、说明里的数字由成员算出、Agent 产出与 Agent 待验收一一对应、趋势合计 = 本周完成）。不需要 v2-server 或数据库。
