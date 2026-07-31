# HubCR 后端指令

[English](AGENTS.md) | **简体中文**

这些指令只作用于 `backend/` 下的文件，并补充根目录 `AGENTS.md`。不得使用本文件
去约束 `frontend/` 或其他兄弟目录的实现方式。

## 后端作用域

后端是一个 Go 模块 `hubcr.io/hubcr`，包含两个可执行进程：

- `cmd/api`：HubCR 业务控制面；
- `cmd/worker`：扫描、签名验证等异步任务。

开始后端工作前，阅读：

- `backend/go.mod` 中当前使用的 Go 版本；
- `docs/architecture.md` 和 `docs/development.md` 的后端部分；
- 已有的相关模块、适配器、迁移和测试。

对于纯后端任务，除非用户明确要求联动修改，否则不要修改 `frontend/`。如果 API
契约发生变化，应报告前端影响，但默认不直接编辑前端。

## 包职责

- `cmd/api` 与 `cmd/worker` 只负责进程启动、信号处理和退出状态。
- `internal/app/controlplane` 组合 API 模块与基础设施适配器。
- `internal/app/worker` 组合任务 Handler 与 Worker 基础设施。
- `internal/modules/<capability>` 拥有业务行为、领域类型、应用服务和该能力使用的
  Port。
- `internal/platform` 拥有配置与具体基础设施适配器。
- `migrations` 拥有 PostgreSQL Schema 演进。

业务模块不得导入 `cmd`、HTTP Handler、进程代码或具体数据库/缓存客户端。
Platform 包实现模块定义的接口，不得作出产品策略决定。跨模块行为通过明确的应用
服务完成，禁止直接访问其他模块的内部存储。

禁止包循环、可变包级全局状态、隐式初始化，以及宽泛的 `common`、`helpers` 或
`utils` 包。代码应放在拥有对应能力的模块中。

## Go 实现规则

- 使用 `go.mod` 声明的 Go 版本，所有代码必须经过 `gofmt`。
- 优先使用标准库，每个新增生产依赖都必须解释和证明必要性。
- 在请求、持久化、网络与 Worker 边界持续传递 `context.Context`。
- 使用 `%w` 为错误增加运行上下文，并保留需要分类的底层错误；禁止比较错误文本。
- 使用 `errors.Is` 或 `errors.As` 进行稳定错误处理。
- `main` 函数、HTTP Handler 和存储适配器保持轻量。
- 接口定义在使用它的模块中，而不是实现它的 Platform 包中。
- 优先使用明确构造函数和依赖注入，避免全局注册表。
- 进程启动时验证配置，并返回可操作的错误。
- 使用结构化 `log/slog` 字段。禁止记录凭据、Token、Authorization Header、
  原始 Cookie、签名密钥或敏感用户数据。
- Server 和 Worker 必须响应取消并优雅退出。
- 禁止启动无上限 Goroutine；每个后台任务都必须有所有者、取消、错误处理和有界
  并发策略。

## HTTP 与 API 规则

- 业务 REST 接口位于 `/api/v1`。
- Registry 认证保留协议专属 `/token` 路径。
- Distribution 负责 `/v2/`，不得把其业务逻辑复制到控制面 Handler。
- Path、Query、Header 和 Body 数据都视为不可信，调用应用服务前必须验证。
- HTTP DTO、领域实体与持久化记录相互分离。
- 使用正确状态码和稳定错误表示，不得返回 SQL、堆栈、内部路径或密钥。
- 可重试命令应尽可能具备幂等性。
- Liveness 只证明进程正在运行；依赖连接后 Readiness 可以检查依赖，但 Liveness
  不能依赖缓慢外部服务。

公开 API 契约变化时，更新测试与受影响文档的中英文版本。除非任务明确跨栈，
否则不要编辑前端实现。

## 持久化与任务

- PostgreSQL 是 HubCR 业务记录的事实来源。
- 必须原子完成的状态变化使用事务。
- 时间使用 UTC 存储，并以明确时区序列化。
- 唯一性、外键等持久化不变量同时在 PostgreSQL 和应用代码中保证。
- 索引服务于已经明确的查询模式，不为猜测添加索引。
- 传输 DTO、领域模型与数据库记录相互分离。
- 第一次公开发布后，迁移只增不改且不可变。
- 初期异步任务使用 PostgreSQL Job Table。任务领取必须原子，Handler 可安全重试，
  失败需要记录，并防止并发重复执行。
- 进程崩溃不能导致已持久化任务静默丢失。

未经明确项目决策，不得选择 ORM、迁移框架或外部消息队列。

## Registry 与安全规则

- 签发作用域 Token 前，按照命名空间、仓库、请求操作和仓库可见性执行授权。
- 用户输入的仓库名经过规范化验证之前不能用于 Token Scope。
- Digest 物理去重不能跨越仓库边界授权访问。
- Tag 可变；Artifact、扫描、SBOM 与验签记录使用不可变 Digest 标识。
- 签名发现、密码学验证与信任策略判断分别保存。
- 实现验签时记录签名 Digest、签名者身份、策略版本和验证时间。
- Registry Event 和 Worker Task 都是不可信输入，必须经过认证或验证、具备幂等性
  并可安全重放。
- 除非已接受策略明确要求，否则扫描与签名任务不能阻塞成功 Push。

## 后端测试

- 单元测试以 `*_test.go` 放在同目录；表驱动测试能提高清晰度时优先使用。
- 测试行为和不变量，不绑定私有实现细节。
- 单元测试必须确定，不得依赖 PostgreSQL、Redis、MinIO、Distribution、网络或
  真实时间等待。
- 集成测试必须明确标记，并提供清晰的初始化和清理流程。
- 每个 Bug 修复都增加回归测试。
- HTTP 行为使用 `httptest`，时间相关逻辑使用可注入 Clock 或时间源。

## 后端验证

后端开发期间在 `backend/` 执行：

```bash
gofmt -w .
go vet ./...
go test ./...
```

宣布整体任务完成前，在仓库根目录运行 `make check`。启动或运行时行为变化时，
执行 API 或 Worker 冒烟测试。如果必要服务不可用，必须明确报告限制。

## 后端代码审查规则

- 标记位于 `cmd`、HTTP Handler 或 `internal/platform` 中的领域行为。
- 标记绕过应用服务的跨模块存储访问。
- 标记跨 I/O 或 Worker 边界丢失的 Context。
- 标记默认允许或接受未验证仓库路径的授权。
- 标记不可幂等、安全重放、取消或限制并发的任务。
- 标记仅按 Tag 保存的 Artifact 安全数据。
- 标记日志与错误响应中的密钥或敏感值。
