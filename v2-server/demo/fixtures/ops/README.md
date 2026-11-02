# 每日巡检演示服务

本项目保存本地演示服务的监测数据，不连接生产系统。

运行 `node scripts/inspect.mjs` 检查 `ops/metrics.json`，输出服务可用性、错误率、磁盘和备份结果。`ops/service.log` 是对应的演示日志。

只读巡检，不修改文件或执行修复。阈值：所有服务 healthy，错误率低于 1%，磁盘低于 80%，最近备份成功。
