# cmd/checkformat: 代码格式检查门禁

[中文](README.md) | [English](README.en.md)

`cmd/checkformat` 是用于本地检查和 CI 流水线的格式验证工具，用来强制执行严格的 Go 代码排版规范。

## 职责

- **非修改式格式检查**：对本模块全部包的目录（`go list ./...`）执行 `go tool gofumpt -l -extra`，列出违反格式规则的文件，但不修改磁盘上的文件。嵌套的 `v2-server` 模块不在检查范围内，由 v2 的工作流单独检查。
- **严格执行门禁**：如果所有 Go 源文件都符合格式规范，程序以状态码 0 退出。如果检测到格式错误，程序会将相应文件名输出到 `stderr`，以状态码 1 退出，并提示开发者运行 `task format`。

## 边界与不变量

- **只读**：`cmd/checkformat` 从不写入或修改源文件。自动格式化由 `task format`（`gofumpt` 和 `goimports`）单独执行。
- **标准化检查**：直接集成到 `task format:check` 和 `task check` 中。

参见 [cmd 入口总览](../README.md) 与 [Taskfile.yml](../../Taskfile.yml)。
