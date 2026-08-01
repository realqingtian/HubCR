# D-006 公开 Pull

[English](d-006-public-pull.md) | **简体中文**

- 状态：`ACCEPTED`
- 批准日期：2026-08-01
- 决策负责人：产品负责人
- 阻塞内容：Registry Token 流程和公开仓库验收场景

## 背景

公开仓库可以允许匿名 Pull，也可以要求认证。该选择会改变 Registry Challenge、
Token 签发、发现和限流行为。

## 决策

明确为 `PUBLIC` 的仓库允许匿名 Pull。Distribution 仍使用 Token 流程：匿名调用者
可以获得只允许对精确公开仓库执行 `pull` 的短期 Token。Push 始终要求调用者已认证
且具有获批能力。可见性或策略数据缺失时默认拒绝。

## 备选方案

- 公开 Pull 也要求认证：更容易关联调用者，但不符合常见公开 Registry 预期。
- 部署时可配置：在默认方案得到验证前就增加两套验收模式。

## 后果

M2 必须测试匿名 Challenge/Token/Pull、私有拒绝、精确 Scope、过期和跨仓库复用。
这些场景成为 M2 的强制验收范围。
