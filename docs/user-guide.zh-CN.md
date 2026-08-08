# HubCR Registry MVP 用户指南

[English](user-guide.md) | **简体中文**

本指南只描述当前 Registry MVP 候选版本已经实现的工作流。不可用能力参见
[发布限制](release-limitations.zh-CN.md)，程序化行为参见 [API 约定](api.zh-CN.md)。

## 登录与导航

打开运维人员提供的 HubCR HTTPS Origin，使用已有本地用户名与密码登录。Web Session
可撤销，且不是 Docker 或 OCI 凭据。Overview 会显示登录用户的个人 Namespace 及其
所属 Organization。

管理员邀请兑换与账号 Bootstrap 尚未形成受支持用户流程。运维人员不得把测试 Seed
命令用于生产账号配置。

## Organization 与 Repository

已认证用户可以创建 Organization。Organization `OWNER` 与 `ADMIN` 可按获批角色矩阵
管理成员；最后一位 `OWNER` 不能被移除或降级。个人 Namespace 及获得授权的组织成员
可以创建 Repository。

每个 Repository 都明确为 `PUBLIC` 或 `PRIVATE`。Visibility 缺失或不可用时绝不会
变为公开。无发现权限的用户访问 Private Repository 时，会获得与 Repository 不存在
完全相同的 Not-found 响应。

## 使用 Docker Pull 与 Push

Repository Detail 根据当前 Visibility 与服务端计算的调用者 `can_pull`、`can_push`
能力显示 Quick-start 命令。不得根据其他位置显示的 Organization Role 自行推导权限。

- 匿名 Client 可以 Pull 明确公开的 Repository。
- Private Pull 要求使用本地用户名与密码执行 `docker login`。
- Push 始终要求认证及已获批 Push 能力。
- Web 浏览器 Session Cookie 不能作为 Registry 凭据。
- Registry Token 生命周期短，只允许精确 Repository 与获批 Action 子集。

运维人员可能使用非默认 DNS 名称或端口，因此必须采用 Repository Detail 显示的精确
Origin 与命令。

## Tag 与 Artifact

Repository Detail 将可变的当前 Tag 与不可变 Artifact 分开列出。选择 Tag 会打开精确
Digest Detail。即使 Tag 后续移动，Digest 仍是 Artifact 身份。

Media Type、Size、创建时间、Index Descriptor 与 Platform 字段只在已知时显示。未知
Descriptor 数据不会被显示为成功的空结果。Loading、Empty、Unavailable、Denied、
Failed、Not-found 与 Success 状态保持区分。

后端会异步记录绑定 Digest 的漏洞扫描、CycloneDX SBOM、Cosign 签名验证与版本化信任
评估。Artifact 安全面板会如实展示经授权状态，而不在客户端作出信任决策。Scan 与
SBOM 状态区分 Queued、Running、Completed、Failed、Unavailable 与 Stale；Signature
则分别表达未签名、密码学无效、未验证、有效但不可信和有效且可信。结果只展示信息，
不会阻断 Pull；缺失证据并不代表安全检查成功。

## 当前安全边界

Repository 删除、Tag 历史、保留、垃圾回收、Robot Account、Personal Access Token、
密码找回、MFA、Audit 导出、配额与公开发现均不可用。维护与恢复请联系运维人员；
不要直接修改 Distribution 存储或 PostgreSQL。
