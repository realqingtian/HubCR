# D-008 Signature trust

**English** | [简体中文](d-008-signature-trust.zh-CN.md)

- Status: `ACCEPTED`
- Approved: 2026-08-08
- Decision owner: product owner
- Blocks: G-03 and M4 signature-verification schema

## Context

Cosign supports both public-key verification and keyless certificate identities.
HubCR needs a durable trust model before M4 persists verification results. Signature
presence, cryptographic validity, and policy trust are different facts; collapsing
them would make later policy changes and historical explanations unreliable.

## Decision

The initial M4 model is one versioned trust policy per namespace, managed
by the organization or personal-namespace owner and inherited by repositories in that
namespace. Repository-specific overrides are deferred.

Each policy can contain either or both of these explicit trust subjects:

1. Public-key records identified by an immutable public-key fingerprint. HubCR stores
   only public verification material and metadata; private signing keys never enter
   HubCR.
2. Keyless identities identified by an exact OIDC issuer and exact certificate
   subject pair.

The first version does not support wildcard identities or implicit trust. Verification
is asynchronous and bound to an artifact digest. A result records the signature
artifact identity, signer evidence, cryptographic-validity state, policy-trust state,
trust-policy version, verification time, and machine-readable reason. Dependency or
transparency-service unavailability is represented separately from invalid and
untrusted results.

Changing a policy creates a new version and queues re-verification; it does not rewrite
historical evidence. This decision defines evidence and trust evaluation only. It does
not authorize pull blocking.

## Alternatives

- One deployment-wide fixed public key: rejected because it cannot express separate
  organization ownership and creates a disruptive schema migration for keyless use.
- Public keys only: rejected because it excludes exact OIDC workload identities used
  by modern signing workflows.
- Keyless identities only: rejected because self-managed and offline public-key
  workflows remain valid deployment needs.
- Repository-specific trust policies in the first version: deferred to avoid policy
  precedence and administration complexity before namespace-level evidence exists.

## Consequences

- M4 schema must separate signature discovery, cryptographic verification, and policy
  trust, all keyed by immutable artifact digest.
- Trust decisions remain explainable after policy rotation because results reference a
  specific policy version and signer evidence.
- UI and APIs must distinguish invalid signatures, valid-but-untrusted signers,
  unavailable verification dependencies, missing signatures, and trusted signatures.
- Namespace owners can use public keys, exact keyless identities, or both, but cannot
  use wildcard subjects in the initial version.
