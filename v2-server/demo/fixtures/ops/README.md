# 每日巡检演示服务

本项目保存本地演示服务的监测数据，不连接生产系统。

运行 `node scripts/inspect.mjs` 检查 `ops/metrics.json`，输出服务可用性、错误率、磁盘和备份结果。`ops/service.log` 是对应的演示日志。

只读巡检，不修改文件或执行修复。阈值：所有服务 healthy，错误率低于 1%，磁盘低于 80%，最近备份成功。

默认样例含错误率、磁盘两项异常，以及已恢复的维护窗口提示。每项 findings 带稳定 key，供跨日关联任务；发现异常仍返回正常采集结果。运行 `node scripts/inspect.mjs --healthy` 可验证无异常样例（findings 为空）。
