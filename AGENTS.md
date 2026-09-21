# 项目约定

- 每次改动代码后，都必须运行打包脚本 `.\build.ps1` 产出可测试的程序包给用户测试（脚本内含 `go test ./...`、`go vet ./...` 与 Windows 便携打包）。需要跳过测试时用 `.\build.ps1 -SkipTests`。
- 构建环境说明：`go` 不在 PATH 时由 build.ps1 自动回退到 `.tools\go\bin\go.exe` 或 `.tools\go\go\bin\go.exe`；GOCACHE/GOMODCACHE 已由脚本指向本仓库 `.tools` 目录。外网模块源不可用，依赖一律使用本地模块缓存。
- **禁止提交（commit）、禁止推送（push）**：除非用户明确要求，否则始终不做版本库写入操作。
- 打包版本号：从本次约定起，每次打包都要更换版本号（在 `internal/httpapi/server.go` 的版本常量、`internal/httpapi/rating_test.go` 的版本断言及 README/VALIDATION 文档中同步），不得沿用旧版本号重复打包。