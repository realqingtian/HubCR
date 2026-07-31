# HubCR 前端指令

[English](AGENTS.md) | **简体中文**

这些指令只作用于 `frontend/` 下的文件，并补充根目录 `AGENTS.md`。不得使用本文件
去约束 `backend/` 或其他兄弟目录的实现方式。

## 以已安装 Next.js 版本为准

当前版本可能与 AI 训练数据中的 Next.js 不同，API、约定和目录结构都可能发生变化。
编写框架代码前必须阅读 `node_modules/next/dist/docs/` 中与任务相关的文档，并遵守
当前弃用说明。

## 前端作用域

前端是用于公开 Registry 页面和登录后 HubCR 工作流的 Next.js Web 应用。它调用
控制面 API，但不实现后端授权、Registry Token 签发、持久化或 Worker 行为。

开始前端工作前，检查：

- `frontend/package.json` 与 `frontend/tsconfig.json`；
- 相关 Route、Feature、API Schema、Provider 和测试；
- 已安装版本的相关 Next.js 文档；
- `docs/architecture.md` 和 `docs/development.md` 的前端部分。

对于纯前端任务，除非用户明确要求联动修改，否则不要修改 `backend/`。如果所需
API 不存在，应定义或记录前端期望并报告后端依赖，不能在客户端伪造替代结果。

## 前端职责

- `app/` 负责 Route、Layout、Metadata、Loading 和 Error Boundary。
- `features/<capability>/` 负责功能 UI、功能 Hook 与功能内部模型。
- `lib/api/` 负责底层类型化 HTTP 访问与 Zod 响应 Schema。
- `providers/` 负责应用级客户端 Provider。
- `public/` 只存放静态资源。

不得在 Route 文件中堆积可复用产品逻辑，不得把功能专属行为放入 `lib`。只有至少
两个真实消费者共享相同语义时，才能提取通用组件或工具。

## Next.js 与 React 规则

- 使用已安装文档中的 App Router 约定。
- Page 和 Layout 默认是 Server Component。
- 只有确实需要状态、事件、Effect、浏览器 API、React Context 或客户端库时，
  才在最小组件边界增加 `"use client"`。
- 不得为了一个交互子组件把整个 Route 或 Layout 移到客户端。
- 跨 Server/Client 边界的 Client Component Props 必须可序列化。
- 初始渲染需要的数据且不依赖客户端会话机制时，优先在服务端获取。
- 用户体验需要时使用 Route 的 `loading`、`error` 与 `not-found` 约定。
- 避免不必要 Effect 和重复派生状态。

## TypeScript 与 API 数据

- 保持 TypeScript Strict，禁止用 `any`、不安全 Cast 或非空断言隐藏契约不匹配。
- API 响应视为不可信，在 `lib/api` 使用 Zod 验证后再交给 Feature。
- 服务端 DTO 与展示模型语义不同时应相互分离。
- 客户端服务端状态、缓存、失效和 Mutation 生命周期使用 TanStack Query，禁止在
  组件状态中复制其缓存。
- Query Key 必须稳定、可序列化，并包含所有会改变结果的输入。
- 请求必须真实区分加载、空数据、不可用、错误和成功状态。
- 不得推测不支持的安全或 Registry 状态；未知与不可用不能显示为成功的零值。
- 禁止通过 `NEXT_PUBLIC_*` 变量或客户端 Bundle 暴露密钥。

## UI、样式与可访问性

- 使用 Tailwind CSS 和现有设计 Token。未经确认不得新增组件库、图标体系或样式
  框架。
- 优先使用语义 HTML，再考虑 ARIA。
- 所有交互控件必须支持键盘、可见焦点和可访问名称。
- 表单输入需要 Label，验证信息必须关联对应字段。
- 保持可读颜色对比，不能只使用颜色表达状态。
- 布局必须兼容移动端与桌面宽度，不能隐藏关键操作。
- 公开与登录后的 UI 必须真实反映后端能力，不得把路线图功能显示为可用控件。

## 功能边界

预期 Feature 包括 `auth`、`organizations`、`namespaces`、`repositories` 和
`security`。只有实现已确认工作流时才创建 Feature 目录。

- Feature 可以依赖 `lib` 和共享 UI，但不能导入无关 Feature 的内部实现。
- 只有抽象能够保持行为且存在多个真实消费者时，才移动到共享位置。
- 前端可以展示授权结果，但不能复制后端授权决定并将其作为安全边界。
- 签名状态必须区分不存在、已发现、密码学有效、策略可信和无效或过期。
- Artifact 页面即使显示 Tag，身份仍使用 Digest。

## 前端测试

- 单元测试与 Schema 测试使用 Vitest。
- 测试与行为放在一起，或使用附近的 `__tests__` 目录。
- 测试可见行为、验证、状态转换和可访问语义，不绑定实现细节。
- 每个前端 Bug 修复增加回归测试。
- 单元测试不得依赖实时后端、网络、计时器或执行顺序。
- 实现完整用户流程后增加 Playwright；单元测试不能证明端到端流程正确。

## 前端文档

前端行为、命令、依赖、Route 或结构变化时，同时更新 `frontend/README.md` 和
`frontend/README.zh-CN.md`。其他受影响文档遵守根目录双语规则。

## 前端验证

本应用使用 Bun。前端开发期间在 `frontend/` 执行：

```bash
bun run typecheck
bun run test
bun run lint
bun run build
```

宣布整体任务完成前，在仓库根目录运行 `make check`。布局、交互、路由、Hydration
或响应式行为变化时必须执行真实浏览器检查；仅构建通过不足以证明这些行为正确。

## 前端代码审查规则

- 标记不必要的 Client Component 和过宽的 `"use client"` 边界。
- 标记未经验证的 API 数据或重复的 TanStack Query 状态。
- 标记 UI 中虚构的授权、扫描、签名或可信结果。
- 标记不可访问的交互控件或缺失的加载、错误、空数据状态。
- 标记放在 Route 或无关共享模块中的 Feature 代码。
- 标记未经明确范围授权就修改后端实现的纯前端工作。
