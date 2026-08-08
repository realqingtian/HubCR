# HubCR decision records

**English** | [简体中文](README.zh-CN.md)

Decision records capture choices that affect durable schema, public APIs,
authorization, security, or operations.

- `PROPOSED`: prepared for review and not authorized for implementation;
- `ACCEPTED`: explicitly approved by the decision owner;
- `SUPERSEDED`: replaced by a later accepted record;
- `REJECTED`: reviewed and not selected.

Implementation must not treat a `PROPOSED` record as policy. Acceptance requires the
product owner to confirm the selected option and approval date.

## M0 decision session

- [D-001 Product mode](d-001-product-mode.md)
- [D-002 Registration mode](d-002-registration.md)
- [D-003 Initial identity](d-003-initial-identity.md)
- [D-004 Organization roles](d-004-organization-roles.md)
- [D-005 Grant inheritance](d-005-grant-inheritance.md)
- [D-006 Public pull](d-006-public-pull.md)

## M3 operations decision session

- [D-009 Production deployment](d-009-production-deployment.md)
- [D-010 Operations policy](d-010-operations-policy.md) — accepted only for the MVP
  backup and restore subset; lifecycle policy remains deferred

## M4 security decision session

- [D-007 Security enforcement](d-007-security-enforcement.md) — `ACCEPTED`:
  informational asynchronous scanning without pull blocking
- [D-008 Signature trust](d-008-signature-trust.md) — `ACCEPTED`: versioned
  namespace policies combining public-key and exact keyless identities
