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
