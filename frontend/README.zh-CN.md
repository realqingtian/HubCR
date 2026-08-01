# HubCR Web 应用

[English](README.md) | **简体中文**

HubCR Web 应用使用 Next.js App Router、React、TypeScript、Tailwind CSS、
TanStack Query 和 Zod。

```bash
bun install
bun run dev
```

应用路由位于 `app`，产品代码按功能组织在 `features`，类型化 API 访问代码位于
`lib/api`，仅客户端使用的全局 Provider 位于 `providers`。

当前控制面工作区支持本地 Session 登录、明确的个人 Namespace、个人与组织
Repository 元数据、组织创建和成员添加。API 响应使用 Zod 校验，服务端状态由
TanStack Query 管理。界面明确区分加载、空数据、校验、拒绝和不可用状态；后端授权
仍然是安全边界。

浏览器请求使用同源 `/api` 路径。在本地开发和独立 Next.js 运行模式下，
`HUBCR_CONTROL_PLANE_URL` 选择服务端 Rewrite 目标，默认值为
`http://127.0.0.1:8080`；部署 Gateway 也可以直接路由 `/api`。
`NEXT_PUBLIC_API_BASE_URL` 仅作为已启用 CORS 端点的可选公开覆盖项，绝不能包含
Secret。

```bash
bun run typecheck
bun run test
bun run lint
bun run build
bun run test:e2e
```

Playwright 套件会构建并启动生产模式 Web Server，再通过浏览器层控制面 Mock 稳定
验证工作流与失败状态。持久化与授权证据由后端 PostgreSQL 集成套件单独验证。

在仓库根目录运行 `make test-m1-e2e` 还会创建隔离 PostgreSQL，通过 GORM 写入仅供
测试的身份，启动真实 Go API 与同源 Next.js Proxy，并在 Chromium 中运行 M1 必需
流程 1–3。`backend/internal/testsupport` 下的夹具不是产品注册或 Bootstrap 入口。
