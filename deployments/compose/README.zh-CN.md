# 本地基础设施

[English](README.md) | **简体中文**

该 Compose 配置用于在本地开发环境启动 PostgreSQL、Redis、MinIO 和 CNCF
Distribution。API、Worker 和 Web 应用不会在 Compose 中启动，以便各进程使用
原生热更新独立运行。

```bash
docker compose --env-file ../../.env.example up -d
```

本地服务地址：

- PostgreSQL：`localhost:5432`
- Redis：`localhost:6379`
- MinIO S3 API：`http://localhost:9000`
- MinIO 控制台：`http://localhost:9001`
- OCI Distribution：`http://localhost:5000`

认证与事件通知目前尚未启用。Registry Token 授权流程确定后，再接入这些能力。
