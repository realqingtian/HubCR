# D-010 运维策略

[English](d-010-operations-policy.md) | **简体中文**

- 状态：MVP 备份与恢复子集为 `ACCEPTED`
- 批准日期：2026-08-08
- 决策负责人：产品负责人
- 阻塞内容：M3-07 备份、恢复与迁移演练

## 背景

完整运维决策包含备份、恢复、保留、删除、垃圾回收、配额、审计和灾难恢复。M3-07
只需要一个有界恢复契约。此时选择无关的破坏性生命周期策略会无声扩大 MVP，并可能
在缺乏产品证据时产生不可逆的数据行为。

## 决策

获批的 MVP 子集如下：

1. 数据备份包含 PostgreSQL 业务数据库，以及 Distribution 使用的 MinIO
   `hubcr-registry` Bucket。
2. Redis 不纳入备份，因为选定 MVP 不在其中保存权威业务状态。
3. Registry 签名私钥、JWKS 轮换材料、Event Token、部署环境和其他 Secret 由运维
   人员另行保护，不包含在普通备份包中。
4. 备份是手动维护窗口操作。运维人员必须先停止 API、Worker、Web、Gateway 与
   Registry 写入，再确认备份。上述 Compose 服务仍在运行时，命令会拒绝执行。
5. 备份带有完整性清单，并以仅 Owner 可访问的权限创建。它包含密码 Hash、Session
   记录、Repository 元数据与 OCI 内容，因此运维人员必须加密并保存到部署宿主机之外。
6. 恢复有意设计为破坏性操作，要求显式确认值，校验每个已记录的 Checksum，替换
   PostgreSQL 与 Registry 对象数据，然后应用全部当前数据库迁移。
7. 恢复后的应用启动前，必须单独提供 Registry Key 与 Secret。演练时轮换签名 Key
   是有效操作，并会让此前签发的短期 Registry Token 失效。
8. 恢复验收必须证明数据库迁移、用户登录、私有 Repository 授权、已有私有镜像
   Pull、Artifact/Tag 可用，以及不可变 Digest 未改变。

自动调度、跨区域灾难恢复、固定 RPO/RTO、备份保留、配额、Repository 删除和
Distribution 垃圾回收仍未获批并保持延期。

## 备选方案

- 对 Docker Volume 做逐字节备份：不采用，因为可移植的 PostgreSQL 逻辑 Dump 与
  S3 层对象复制具有更清晰的版本和完整性边界。
- 把 Registry Key 与部署 Secret 放入同一归档：不采用，因为这会让数据恢复绑定到
  单个高价值 Secret 包，并扩大被盗后的影响。
- 在 Push 与业务写入继续时在线备份：不采用，因为 PostgreSQL Snapshot 与单独复制
  的对象存储可能代表不同的 Repository 状态。
- 现在定义固定 RPO/RTO 与保留策略：延期到获得真实部署需求和存储成本数据之后。

## 后果

- MVP 恢复流程需要停机，不属于高可用设计。
- 本地演练成功只是发布证据，不代表异地灾难恢复已得到证明；运维人员仍须单独验证
  加密存储及 TLS/Secret 恢复流程。
- 后续删除、保留、垃圾回收、配额与审计工作仍需显式批准，不得从本记录推导策略。
