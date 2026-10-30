# 无项目文本单轮执行器

`modelturn.Run` 在调用方完成 admission 后，通过后端配置的 `nodeservices.Binding` 执行一次纯文本模型调用。它同步调用 `Provider.Complete`，使用进程内 `http.ResponseWriter` 解析 OpenAI 兼容 JSON / SSE，不启动 HTTP 监听器、Node 或 Workspace，也不重试或接续截断回答。

```go
result, err := modelturn.Run(ctx, binding, []modelturn.Message{
    {Role: "user", Content: "上一问"},
    {Role: "assistant", Content: "上一答"},
    {Role: "user", Content: "当前问题"},
}, func(ctx context.Context, delta string) error {
    return persistTextDelta(ctx, delta)
})
// 调用方继续持久化 result；Run 本身不表示 admission、片段或终态已经入库。
```

`Message` 只接受 `user` / `assistant` 文本，首尾必须是 `user`。历史会话和文本来源的鉴权、排序与筛选由调用方负责；不能把任意页面输入变成可信系统指令。模型只来自 `binding.Model`，请求固定为一个 choice、流式文本、请求 usage，且不包含工具、系统消息、客户端凭据或可覆盖的 provider 参数。

`Emit` 按顺序同步接收完整 UTF-8 文本片段，可施加背压。失败会立即取消派生的 provider context，并返回固定的 `model_event_write_failed`，不暴露回调错误正文；失败片段不计入返回的 `Answer`。Provider 和 Emit 都必须遵守 context；Provider 必须同步使用 writer，并在返回前结束写入。Run 等待 Provider 调用退出，自己不遗留读取 goroutine。SSE 正常停止并收到完整 `[DONE]` 后主动取消上游 I/O，再等待 Provider 退出，不因上游保持连接而等到超时；这次主动收尾保留 completed 结果。调用方在完成前取消执行 context 会停止模型调用；观察者断连不应取消该 context。

## 完整性和限制

JSON 必须是一个完整对象，只有一个 `index: 0` choice，明确包含 assistant 字符串 content 和 `finish_reason: "stop"`。SSE 支持跨写入分片、CR / LF / CRLF、多个 data 行及 Unicode；每个 data 事件必须完整闭合。只在整个流开头忽略一次 UTF-8 BOM，包括 BOM 自身跨写入分片的情况；重复、错位或损坏的 BOM 不能使 data 字段被静默忽略。SSE 行须为有效 UTF-8，字段名限定 ASCII，正文中的 U+FEFF 保持原样。整个流必须明确提供至少一次字符串 content，正常 finish 后再收到完整 `[DONE]` 事件才可成功。

SSE 接受 role-only、空终态 delta、仅有 finish_reason 的收尾 choice，以及任何位置不带 error 的空 `choices` 元数据块（例如 Azure 首块过滤信息、usage-only chunk）。空字符串 finish_reason 与 null 一样表示尚未结束；它不能替代正常 stop。已收到的终态后新 choice、同次写入中 DONE 后的新 data 仍失败；完成收尾后停止继续读取上游。明确提供 `content:""` 并正常结束时，空回答算完整；只有 role、reasoning 或元数据而没有字符串 content 时不算完成。

JSON / SSE payload 经 `strictjson.Valid` 验证，拒绝无效 UTF-8、孤立 UTF-16 surrogate、损坏 JSON。合法 surrogate pair、实际 U+FFFD 和普通反斜杠文本保持原样。未知的 message / delta 扩展字段会被忽略，包括 `reasoning_content` 和 `reasoning`；只有 `content` 会发布为答案，推理文本不能补足缺失的答案。已支持字段仍校验类型；工具调用、拒绝、非文本内容与未知停止原因均不能被当作完整文本回答。上游非 2xx、错误 envelope 与未知响应格式只产生固定错误码，不发布原始错误正文。

标准纯文本元数据兼容 [ChatCompletionMessage 定义](https://developers.openai.com/api/reference/resources/chat)：JSON message / SSE delta 的 `annotations` 可以缺失、null 或空数组，`audio` 可以缺失或 null。非空 annotations 或实际 audio 明确返回 `model_response_unsupported`，不静默丢弃已知的附加内容。

固定限制按字节计算：最多 256 条 messages；序列化请求最多 1 MiB；整个响应最多 64 MiB；SSE 单行及单个 data 事件缓冲最多 8 MiB；累积答案最多 16 MiB。执行最多 5 分钟，也受调用方更早的 deadline 约束。超限立即失败，不自动截短后报告成功。

| 结果 | Status | StopReason / FailureCode |
| --- | --- | --- |
| 有正常结束证据 | `completed`，`AnswerComplete=true` | `stop` |
| 调用方取消 | `cancelled` | `cancelled` |
| deadline 到期 | `failed` | `model_timeout` |
| 长度截断 | `failed` | `length` / `model_output_truncated` |
| 拒绝或内容过滤 | `failed` | `refusal` 或 `content_filter` / `model_refused` |
| 工具或 function call | `failed` | `tool_calls` 或 `function_call` / `model_response_unsupported` |
| 缺终态、损坏、超限、provider 或 Emit 失败 | `failed` | 固定的 `model_*` 错误码 |

所有失败均 `AnswerComplete=false`，可能保留已经 Emit 成功的部分文本。`StopReason: "stop"` 单独不能表示完成，例如 SSE 缺少 DONE 时仍然失败。终态持久化、事件序号、幂等、历史上下文、租约、重启恢复和观察者重连由 execution 调用方负责。

## Usage

依据本地官方 `@agentclientprotocol/sdk` 的 `Usage` 类型，只有 provider 明确提供完整的 `prompt_tokens`、`completion_tokens`、`total_tokens` 时，才输出 `inputTokens`、`outputTokens`、`totalTokens`；缺失时返回 nil，不补零。计数类型必须是 JavaScript 安全整数范围内的非负整数；完整计数的总数不相等时仅将 usage 留空，不使完整回答失败。后到的空或不完整 usage 元数据不会清掉此前有效计数；后到的完整但不一致计数则清空 usage。显式提供的 `reasoning_tokens` / `cached_tokens` 分别映射 `thoughtTokens` / `cachedReadTokens`；没有来源的 `cachedWriteTokens` 不生成。这里是本次调用的计数，ACP 会话累计由调用方处理。

## 验证

`go test -race ./internal/modelturn` 覆盖 FixtureProvider、真实 `httptest` TLS + RemoteProvider 的 JSON / SSE、推理扩展与过滤元数据、finish-only 收尾、usage 兼容、DONE 后连接不关闭时的主动取消和 join、上下文取消和 Emit 失败、Unicode / 任意字节分片、多 data 行、缺 DONE、损坏或超限响应、工具 / 拒绝及凭据 / 错误正文不泄漏。取消测试通过 channel 同步证据，不依赖 sleep；测试不会调用外部付费模型或启动数据库。
