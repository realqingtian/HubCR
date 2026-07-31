# HubCR repository instructions

The canonical repository rules are in [`AGENTS.md`](../AGENTS.md). Read and follow
that file before proposing, reviewing, or changing code. Also follow any nearer
`AGENTS.md` file for the path being changed.

In particular:

- preserve the Go modular-monolith control plane and CNCF Distribution data-plane
  boundary;
- preserve repository-scoped authorization and digest-based security invariants;
- do not invent unresolved product policies;
- update English and Simplified Chinese documentation together;
- add focused tests for behavior changes and run `make check` before completion;
- never commit secrets or perform Git/external mutations unless explicitly requested.
