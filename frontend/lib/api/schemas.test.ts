import { describe, expect, it } from "vitest";
import {
  artifactDetailSchema,
  artifactListSchema,
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
