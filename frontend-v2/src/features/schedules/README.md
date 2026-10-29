# 定时任务

列表与右侧抽屉支持保存、编辑、暂停和恢复无项目文本任务配置，配置通过 v2-server 持久化，刷新后保留。工作流、项目、模型选择、执行、删除和历史尚未接入；界面不会生成模拟运行结果。

时间规则支持每天、工作日、每周及固定小时间隔；日历规则按任务时区预览，间隔以创建时间为起点。预览不代表调度已启用。

临时 v2 配置接口由 `v2-server/schedules.openapi.json` 描述，使用 `npm run api:generate:schedules` 生成本功能客户端；正式 Cloud 的接口缺口见 v2-server README。会话和租户继续通过正式生成客户端读取。

`npm run check` 运行类型检查、lint、单元测试和构建。`npm run e2e` 使用真实 v2-server 与 PostgreSQL 验证页面；可通过 `E2E_DATABASE_URL` 和 `PW_CHROMIUM_PATH` 指定数据库与浏览器。
