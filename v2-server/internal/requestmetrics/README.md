# HTTP 请求观测

为一个可信执行轮次包装 provider，派发前持久化一次 attempt，之后原样转发响应并有界观察 JSON/SSE。只提取允许的状态、协议、时长、首个有效输出及供应商用量数字；不保存 payload、header、token 或凭据。

每次请求单独计数；finish 重试复用同一 ID 和完整结果，不重发模型调用。started 不代表仍活跃，也可能是收尾写入缺失。缺值保持 null，真实零保持零；完整协议结束与每项指标覆盖率分别判断。fixture 来源与上游请求单独标记。

`usage.go` 的解析语义从现有 Cloud Gateway 迁入，回归覆盖 OpenAI/OpenRouter/Anthropic、缺失与非法值、最终用量帧和有效输出识别。`recorder_test.go` 检查登记失败不派发、CRLF 分片、转发字节保真与收尾重试不重复调用 provider。
