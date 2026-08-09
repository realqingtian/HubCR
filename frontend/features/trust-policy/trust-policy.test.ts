import { afterEach, describe, expect, it, vi } from "vitest";
import {
  createTrustPolicyVersion,
  getCurrentTrustPolicy,
} from "../../lib/api/client";
import {
  createTrustPolicyRequestSchema,
  trustPolicySchema,
} from "../../lib/api/schemas";

const policyID = "44444444-4444-4444-8444-444444444444";
const userID = "11111111-1111-4111-8111-111111111111";

const trustPolicy = {
  id: policyID,
  version: 2,
  public_keys: [
    {
      name: "release",
      fingerprint: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      public_key_pem: "-----BEGIN PUBLIC KEY-----\nMFkwEw==\n-----END PUBLIC KEY-----\n",
    },
  ],
  keyless_identities: [
    { issuer: "https://token.actions.githubusercontent.com", subject: "https://github.com/acme/app/.github/workflows/release.yml@refs/heads/main" },
  ],
  created_by_user_id: userID,
  created_at: "2026-08-09T05:00:00Z",
};

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("trust policy schema", () => {
  it("parses a policy with public keys and keyless identities", () => {
    expect(trustPolicySchema.parse(trustPolicy).version).toBe(2);
  });

  it("parses a policy with no subjects", () => {
    const parsed = trustPolicySchema.parse({
      ...trustPolicy,
      public_keys: [],
      keyless_identities: [],
    });
    expect(parsed.public_keys).toHaveLength(0);
    expect(parsed.keyless_identities).toHaveLength(0);
  });

  it("rejects a malformed fingerprint", () => {
    expect(() =>
      trustPolicySchema.parse({ ...trustPolicy, public_keys: [{ ...trustPolicy.public_keys[0], fingerprint: "not-a-fingerprint" }] }),
    ).toThrow();
  });

  it("rejects a non-https keyless issuer", () => {
    expect(() =>
      trustPolicySchema.parse({ ...trustPolicy, keyless_identities: [{ issuer: "http://insecure.example", subject: "s" }] }),
    ).toThrow();
  });
});

describe("create trust policy request schema", () => {
  it("requires at least one trust subject", () => {
    expect(() => createTrustPolicyRequestSchema.parse({})).toThrow();
    expect(() => createTrustPolicyRequestSchema.parse({ public_keys: [], keyless_identities: [] })).toThrow();
  });

  it("accepts a single public key", () => {
    const parsed = createTrustPolicyRequestSchema.parse({ public_keys: trustPolicy.public_keys });
    expect(parsed.public_keys).toHaveLength(1);
    expect(parsed.keyless_identities).toBeUndefined();
  });

  it("accepts a single keyless identity", () => {
    const parsed = createTrustPolicyRequestSchema.parse({ keyless_identities: trustPolicy.keyless_identities });
    expect(parsed.keyless_identities).toHaveLength(1);
    expect(parsed.public_keys).toBeUndefined();
  });
});

describe("trust policy client", () => {
  it("encodes the namespace path on read", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify(trustPolicy), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const result = await getCurrentTrustPolicy("platform team");
    expect(result.version).toBe(2);
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/namespaces/platform%20team/trust-policy",
      expect.objectContaining({ credentials: "include" }),
    );
  });

  it("posts the create body as JSON and returns the new version", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ ...trustPolicy, version: 3 }), {
        status: 201,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const result = await createTrustPolicyVersion("platform-team", {
      public_keys: trustPolicy.public_keys,
    });
    expect(result.version).toBe(3);
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/namespaces/platform-team/trust-policy",
      expect.objectContaining({
        method: "POST",
        credentials: "include",
        body: JSON.stringify({ public_keys: trustPolicy.public_keys }),
        headers: { "Content-Type": "application/json" },
      }),
    );
  });

  it("surfaces a 404 read as a not_found API error", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({ error: { code: "not_found", message: "resource not found" }, request_id: "req-1" }),
        { status: 404, headers: { "Content-Type": "application/json" } },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(getCurrentTrustPolicy("platform-team")).rejects.toMatchObject({
      status: 404,
      code: "not_found",
      requestID: "req-1",
    });
  });
});
