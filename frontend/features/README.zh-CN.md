# 前端功能模块

[English](README.md) | **简体中文**

功能代码按产品能力组织在该目录中，初始边界包括：

- `auth`：注册、登录、会话与 OIDC 流程
- `organizations`：组织与成员关系
- `namespaces`：个人与组织命名空间
- `repositories`：可见性、权限、Tag 与 Artifact
- `security`：扫描报告、签名与可信状态

每个 MVP 工作流确认后再创建对应目录。路由文件保留在 `app`，共享 API 代码保留
在 `lib`，应用级 Provider 保留在 `providers`。
