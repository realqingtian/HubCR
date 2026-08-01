import { describe, expect, it } from "vitest";
import {
  healthResponseSchema,
  namespaceNameSchema,
  repositorySchema,
  userSchema,
} from "./schemas";

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
  });
});
