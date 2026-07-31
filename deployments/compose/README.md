# Local infrastructure

This Compose stack starts PostgreSQL, Redis, MinIO, and CNCF Distribution for local
development. It intentionally does not start the API, worker, or web application so
each process can run with native hot reload.

```bash
docker compose --env-file ../../.env.example up -d
```

The local endpoints are:

- PostgreSQL: `localhost:5432`
- Redis: `localhost:6379`
- MinIO S3 API: `http://localhost:9000`
- MinIO console: `http://localhost:9001`
- OCI Distribution: `http://localhost:5000`

Authentication and event notifications are not enabled yet. They will be connected
after the Registry Token authorization flow is specified.
