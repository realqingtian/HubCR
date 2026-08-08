# HubCR 决策记录

[English](README.md) | **简体中文**

决策记录用于保存会影响持久化 Schema、公共 API、授权、安全或运维的选择。

- `PROPOSED`：已准备供审查，尚未授权实现；
- `ACCEPTED`：已由决策负责人明确批准；
- `SUPERSEDED`：已被后续获批记录取代；
- `REJECTED`：已审查但未选择。

实现不得将 `PROPOSED` 记录视为已确定策略。产品负责人确认选项和批准日期后，记录
才会生效。

## M0 决策会议

- [D-001 产品模式](d-001-product-mode.zh-CN.md)
- [D-002 注册模式](d-002-registration.zh-CN.md)
- [D-003 首期身份](d-003-initial-identity.zh-CN.md)
- [D-004 组织角色](d-004-organization-roles.zh-CN.md)
- [D-005 权限继承](d-005-grant-inheritance.zh-CN.md)
- [D-006 公开 Pull](d-006-public-pull.zh-CN.md)

## M3 运维决策会议

- [D-009 生产部署](d-009-production-deployment.zh-CN.md)
- [D-010 运维策略](d-010-operations-policy.zh-CN.md) — 仅 MVP 备份与恢复子集获批；
  生命周期策略仍保持延期

## M4 安全决策会议

- [D-007 安全执行策略](d-007-security-enforcement.zh-CN.md) — `ACCEPTED`：
  异步信息展示，不阻断 Pull
- [D-008 签名信任](d-008-signature-trust.zh-CN.md) — `ACCEPTED`：组合公钥和精确
  无密钥身份的版本化 Namespace 策略
