import { describe, expect, it } from "vitest";
import {
  artifactDetailSchema,
  artifactListSchema,
  artifactSecuritySchema,
  healthResponseSchema,
  namespaceNameSchema,
  repositoryDetailSchema,
  repositorySchema,
  tagListSchema,
  userSchema,
} from "./schemas";

const artifactDigest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";
const childDigest = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb";
const timestamp = "2026-08-01T12:00:00Z";

describe("healthResponseSchema", () => {
  it("accepts the control-plane health response", () => {
    expect(healthResponseSchema.parse({ status: "ok" })).toEqual({ status: "ok" });
  });

  it("rejects an unknown health state", () => {
    expect(() => healthResponseSchema.parse({ status: "unknown" })).toThrow();
  });
});

describe("control-plane product schemas", () => {
  it("validates route path components with the canonical OCI name rule", () => {
    expect(namespaceNameSchema.parse("platform-team")).toBe("platform-team");
    expect(() => namespaceNameSchema.parse("Platform Team")).toThrow();
    expect(() => namespaceNameSchema.parse("nested/repository")).toThrow();
  });
  it("requires an explicit personal namespace on authenticated users", () => {
    const user = {
      id: "11111111-1111-4111-8111-111111111111",
      username: "Owner",
      personal_namespace: "owner",
      created_at: "2026-08-01T12:00:00Z",
    };
    expect(userSchema.parse(user)).toEqual(user);
    expect(() => userSchema.parse({ ...user, personal_namespace: undefined })).toThrow();
  });

  it("keeps repository visibility explicit and canonical", () => {
    const repository = {
      id: "22222222-2222-4222-8222-222222222222",
      namespace: "platform-team",
      name: "backend",
      visibility: "PRIVATE",
      description: "images",
      created_by_user_id: "11111111-1111-4111-8111-111111111111",
      visibility_updated_by_user_id: "11111111-1111-4111-8111-111111111111",
      visibility_updated_at: "2026-08-01T12:00:00Z",
      created_at: "2026-08-01T12:00:00Z",
      updated_at: "2026-08-01T12:00:00Z",
    };
    expect(repositorySchema.parse(repository)).toEqual(repository);
    expect(() => repositorySchema.parse({ ...repository, visibility: undefined })).toThrow();
    expect(() => repositorySchema.parse({ ...repository, visibility: "INTERNAL" })).toThrow();
    expect(repositoryDetailSchema.parse({
      ...repository,
      capabilities: { can_pull: true, can_push: false },
    }).capabilities).toEqual({ can_pull: true, can_push: false });
    expect(() => repositoryDetailSchema.parse(repository)).toThrow();
  });
});

describe("artifact and tag schemas", () => {
  const baseArtifact = {
    digest: artifactDigest,
    media_type: "application/vnd.oci.image.manifest.v1+json",
    size_bytes: 512,
    discovered_at: timestamp,
    updated_at: timestamp,
  };

  it("accepts a Manifest without inventing Index descriptors", () => {
    const artifact = { ...baseArtifact, kind: "MANIFEST", descriptors_complete: false };
    expect(artifactDetailSchema.parse(artifact)).toEqual(artifact);
  });

  it("distinguishes unknown, confirmed-empty, and populated Index descriptor sets", () => {
    const unknown = { ...baseArtifact, kind: "INDEX", descriptors_complete: false };
    const confirmedEmpty = { ...baseArtifact, kind: "INDEX", descriptors_complete: true, manifests: [] };
    const populated = {
      ...confirmedEmpty,
      manifests: [{
        position: 0,
        digest: childDigest,
        platform: { os: "linux", architecture: "arm64", variant: "v8" },
      }],
    };

    expect(artifactDetailSchema.parse(unknown)).toEqual(unknown);
    expect(artifactDetailSchema.parse(confirmedEmpty)).toEqual(confirmedEmpty);
    expect(artifactDetailSchema.parse(populated)).toEqual(populated);
  });

  it("rejects contradictory descriptor knowledge and invalid digest identity", () => {
    expect(() => artifactDetailSchema.parse({
      ...baseArtifact,
      kind: "INDEX",
      descriptors_complete: true,
    })).toThrow();
    expect(() => artifactDetailSchema.parse({
      ...baseArtifact,
      kind: "MANIFEST",
      descriptors_complete: false,
      manifests: [],
    })).toThrow();
    expect(() => artifactDetailSchema.parse({
      ...baseArtifact,
      digest: "sha256:not-a-digest",
      kind: "MANIFEST",
      descriptors_complete: false,
    })).toThrow();
  });

  it("validates Artifact and Tag list envelopes without treating Tags as identity", () => {
    const artifact = { ...baseArtifact, kind: "MANIFEST", descriptors_complete: false };
    expect(artifactListSchema.parse({ items: [artifact], meta: { limit: 100 } }).items[0]?.digest).toBe(artifactDigest);
    expect(tagListSchema.parse({
      items: [{ name: "latest", digest: artifactDigest, created_at: timestamp, updated_at: timestamp }],
      meta: { limit: 100 },
    }).items[0]?.name).toBe("latest");
    expect(() => tagListSchema.parse({
      items: [{ name: "bad/tag", digest: artifactDigest, created_at: timestamp, updated_at: timestamp }],
      meta: { limit: 100 },
    })).toThrow();
  });
});

describe("artifact security schema", () => {
  const result = { state: "COMPLETED", attempts: 1, updated_at: timestamp, completed_at: timestamp };
  const policyID = "33333333-3333-4333-8333-333333333333";

  it("accepts absent, unsigned, invalid, untrusted, and trusted backend states", () => {
    const evidence = [
      {
        kind: "SIGNATURE", signature_digest: artifactDigest, signer_type: "PUBLIC_KEY",
        key_fingerprint: childDigest, cryptographic_state: "VALID", trust_state: "TRUSTED",
        reason: "TRUSTED_PUBLIC_KEY",
      },
      {
        kind: "SIGNATURE", signature_digest: childDigest, signer_type: "KEYLESS",
        oidc_issuer: "https://issuer.example", subject: "release@example.com",
        cryptographic_state: "VALID", trust_state: "UNTRUSTED", reason: "VALID_UNTRUSTED_SIGNER",
      },
      {
        kind: "ATTESTATION", signature_digest: "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
        signer_type: "PUBLIC_KEY", key_fingerprint: childDigest,
        cryptographic_state: "INVALID", trust_state: "NOT_EVALUATED", reason: "SIGNATURE_INVALID",
      },
    ];
    const completed = artifactSecuritySchema.parse({
      digest: artifactDigest,
      scan: { ...result, tool: { name: "TRIVY", scanner_version: "0.72.0", database_schema_version: 2, database_updated_at: timestamp, database_downloaded_at: timestamp }, finding_count: 1, severity_counts: { HIGH: 1 } },
      sbom: { ...result, format: "CYCLONEDX_JSON" },
      signature: { ...result, policy_id: policyID, policy_version: 2, cosign_version: "v3.0.6", evidence },
    });
    expect(completed.signature.evidence).toHaveLength(3);
    expect(artifactSecuritySchema.parse({
      ...completed,
      signature: { state: "ABSENT", evidence: [] },
    }).signature.state).toBe("ABSENT");
    expect(artifactSecuritySchema.parse({
      ...completed,
      signature: { ...result, policy_id: policyID, policy_version: 2, cosign_version: "v3.0.6", evidence: [] },
    }).signature.evidence).toEqual([]);
  });

  it("rejects contradictory signer, validity, trust, and absent states", () => {
    const completedScan = {
      ...result,
      tool: { name: "TRIVY", scanner_version: "0.72.0", database_schema_version: 2, database_updated_at: timestamp, database_downloaded_at: timestamp },
      finding_count: 0,
      severity_counts: {},
    };
    const base = {
      digest: artifactDigest,
      scan: completedScan,
      sbom: { ...result, format: "CYCLONEDX_JSON" },
      signature: { ...result, policy_id: policyID, policy_version: 2, cosign_version: "v3.0.6", evidence: [] },
    };
    expect(() => artifactSecuritySchema.parse({
      ...base,
      signature: { state: "ABSENT", policy_id: policyID, evidence: [] },
    })).toThrow();
    expect(() => artifactSecuritySchema.parse({
      ...base,
      signature: { ...base.signature, evidence: [{
        kind: "SIGNATURE", signature_digest: artifactDigest, signer_type: "UNKNOWN",
        cryptographic_state: "VALID", trust_state: "TRUSTED", reason: "bad",
      }] },
    })).toThrow();
  });

  it("rejects completed or failed states whose required evidence is missing", () => {
    const base = {
      digest: artifactDigest,
      scan: { state: "QUEUED", attempts: 0, updated_at: timestamp },
      sbom: { state: "QUEUED", attempts: 0, updated_at: timestamp },
      signature: { state: "ABSENT", evidence: [] },
    };
    expect(() => artifactSecuritySchema.parse({ ...base, scan: result })).toThrow();
    expect(() => artifactSecuritySchema.parse({ ...base, sbom: result })).toThrow();
    expect(() => artifactSecuritySchema.parse({
      ...base,
      scan: { state: "FAILED", attempts: 3, updated_at: timestamp },
    })).toThrow();
    expect(() => artifactSecuritySchema.parse({
      ...base,
      signature: { state: "FAILED", attempts: 3, updated_at: timestamp, policy_id: policyID, policy_version: 2, evidence: [] },
    })).toThrow();
  });
});
