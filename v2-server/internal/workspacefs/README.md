# workspacefs

只读查看 Agent 的工作副本：一层目录、一个文本文件、相对克隆时 commit 的变更列表和单文件 diff。路由与行为见 [v2-server README 的「Agent 工作区查看」](../../README.md#agent-工作区查看)。

- 页面传入的路径先经 `Clean` 规范化（拒绝绝对路径、反斜杠、NUL 和 `.git`），再在解析符号链接后确认仍在工作副本内。
- 变更和 diff 用 index 的临时副本加 `git add --intent-to-add --all` 让未跟踪文件可见，不写 Agent 的 index。
- 所有读取都有上限（`MaxFileBytes`、`MaxDiffBytes`、`MaxEntries`、`MaxChanges`），超出返回 `ErrTooLarge` 或标记 `Truncated`。
