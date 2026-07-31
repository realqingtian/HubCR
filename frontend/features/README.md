# Frontend features

**English** | [简体中文](README.zh-CN.md)

Feature code is grouped here by product capability. Initial boundaries are:

- `auth`: registration, login, sessions, and OIDC flows
- `organizations`: organizations and memberships
- `namespaces`: personal and organization namespaces
- `repositories`: visibility, permissions, tags, and artifacts
- `security`: scan reports, signatures, and trust status

The directories will be created as each MVP workflow is specified. Route files remain
in `app`, shared API code remains in `lib`, and application-wide providers remain in
`providers`.
