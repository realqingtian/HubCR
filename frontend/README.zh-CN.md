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
