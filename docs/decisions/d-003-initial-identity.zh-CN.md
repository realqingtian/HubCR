# D-003 首期身份

[English](d-003-initial-identity.md) | **简体中文**

- 状态：`ACCEPTED`
- 批准日期：2026-08-01
- 决策负责人：产品负责人
- 阻塞内容：凭据、会话、登录 API 与登录 UI

## 背景

本地凭据不依赖外部身份提供方。OIDC 可以减少本地密码处理，但无法独立完成所有
自托管安装的 Bootstrap。

## 决策

MVP 默认使用本地用户名/密码。只存储现代的带 Salt 密码 Hash，签发可撤销的服务端
Web Session，并让认证边界可在以后接入 OIDC Provider。

## 备选方案

- 仅 OIDC：减少密码范围，但强制依赖外部 Provider。
- 同时支持本地凭据和 OIDC：扩大首期账号关联契约。

## 后果

M1 需要密码 Hash、Secret 安全的登录错误、Session 过期/撤销以及可接入限流的端点。
邮件验证、恢复、MFA 和 OIDC 保持不可用。
