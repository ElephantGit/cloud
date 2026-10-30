# 定时触发历史 HTTP

候选契约为 [schedule-history.openapi.json](schedule-history.openapi.json)。与配置契约独立，避免更改配置客户端；不修改正式 Cloud OpenAPI。两条路由均使用现有登录 cookie：

| 请求 | 响应 |
|---|---|
| `GET /api/v2/tenants/{tid}/schedule-configs/{id}/runs` | `{items, nextCursor}`，空列表为 `[]`，末页游标为空字符串 |
| `GET /api/v2/tenants/{tid}/schedule-configs/{id}/runs/{oid}` | 单条持久 occurrence，包括冻结配置、状态、原因、执行引用、释放标记与版本 |

每次读取检查当前成员关系、用户/租户有效性及配置创建者归属。详情同时校验任务与记录关系；无权限、任务或记录不存在均返回 `404 not_found`。缺失/失效登录 cookie 为 `401 unauthenticated`。历史存储未注入为 `503 schedule_history_unavailable`，存储错误为 `500 internal`，不向客户端返回数据库错误。历史响应禁止缓存。

## 分页

`limit` 默认 20，范围 1–100；`cursor` 应使用上页原样返回的值。游标绑定租户、用户和任务；非法编码、跨作用域游标、重复或未知参数均为 `400 invalid_pagination`。详情不接受分页参数。

排序沿用记录底座的不可变 UUID 降序，**不表示时间倒序**。单页授权和读取处于同一个只读数据库快照，用额外一条记录判断是否有下一页，不返回总数。各页是不同数据库快照；翻页期间新增记录可能在当前游标之前，刷新首屏开始新遍历。状态更新不会改变 ID 或排序位置。游标只是位置，不是授权凭证；篡改位置不会绕过数据库权限检查。

## 状态与执行边界

接口原样返回 `recorded`、`starting`、`admission_unknown`、`reconciling`、`running`、`cancelling`、`succeeded`、`failed`、`cancelled`、`interrupted`、`skipped`。`recorded` 仅表示触发已记录；`released` 表示该记录不再占用同任务执行资格，不能单独当作执行成功或结束的证据。结果终态仍可能是 `released=false`，需要等待执行侧清理确认。跳过原因与状态分开提供。

读取不会触发、取消、Seal、恢复核对或修改状态。最终回答、工具事件、公共会话及其持久化仍由公共执行模块承担，当前接口只返回其不透明引用，不伪造回答或开始/结束时刻。配置接口的 `runs` 字段仍为空，历史需通过本接口单独读取。没有新增前端、立即运行或取消入口，生产扫描与真实派发仍未装配。

真实 PostgreSQL HTTP 测试校验候选契约、全部状态及清理标记、空列表/分页/游标作用域、跨用户/租户/任务隔离、成员撤销、用户及租户停用、服务不可用、错误脱敏、服务替换和连接池重开后读取。
