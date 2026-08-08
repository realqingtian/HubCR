import { afterEach, describe, expect, it, vi } from "vitest";
import {
  getArtifact,
  getCurrentUser,
  getRepository,
  listArtifacts,
  listTags,
  login,
  updateRepository,
} from "./client";

const user = {
  id: "11111111-1111-4111-8111-111111111111",
  username: "owner",
  personal_namespace: "owner",
  created_at: "2026-08-01T12:00:00Z",
};

const repository = {
  id: "22222222-2222-4222-8222-222222222222",
  namespace: "platform-team",
  name: "backend",
  visibility: "PUBLIC",
  description: "images",
  created_by_user_id: user.id,
  visibility_updated_by_user_id: user.id,
  visibility_updated_at: "2026-08-01T12:00:00Z",
  created_at: "2026-08-01T12:00:00Z",
  updated_at: "2026-08-01T12:00:00Z",
};

const artifactDigest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";
const artifact = {
  digest: artifactDigest,
  kind: "MANIFEST",
  media_type: "application/vnd.oci.image.manifest.v1+json",
  size_bytes: 512,
  descriptors_complete: false,
  discovered_at: "2026-08-01T12:00:00Z",
  updated_at: "2026-08-01T12:00:00Z",
};

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("typed API client", () => {
  it("sends credentials and validates login responses", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ user, expires_at: "2026-08-02T12:00:00Z" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const result = await login("owner", "correct password");

    expect(result.user.personal_namespace).toBe("owner");
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/auth/login",
      expect.objectContaining({ method: "POST", credentials: "include" }),
    );
  });

  it("returns structured API errors without trusting invalid success bodies", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          error: { code: "authentication_failed", message: "authentication failed" },
          request_id: "request-1",
        }),
        { status: 401, headers: { "Content-Type": "application/json" } },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(getCurrentUser()).rejects.toMatchObject({
      status: 401,
      code: "authentication_failed",
      requestID: "request-1",
    });
  });

  it("encodes repository paths and validates mutation responses", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify(repository), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const result = await updateRepository("platform.team", "backend_api", { visibility: "PUBLIC" });

    expect(result.visibility).toBe("PUBLIC");
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/namespaces/platform.team/repositories/backend_api",
      expect.objectContaining({ method: "PATCH", credentials: "include" }),
    );
  });

  it("reads and validates repository detail routes", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({
        ...repository,
        capabilities: { can_pull: true, can_push: false },
      }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const result = await getRepository("platform-team", "backend");

    expect(result.name).toBe("backend");
    expect(result.capabilities).toEqual({ can_pull: true, can_push: false });
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/namespaces/platform-team/repositories/backend",
      expect.objectContaining({ credentials: "include" }),
    );
  });

  it("reads repository-scoped Artifact and Tag discovery routes", async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ items: [artifact], meta: { limit: 100 } }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }))
      .mockResolvedValueOnce(new Response(JSON.stringify({
        items: [{
          name: "latest",
          digest: artifactDigest,
          created_at: "2026-08-01T12:00:00Z",
          updated_at: "2026-08-01T12:00:00Z",
        }],
        meta: { limit: 100 },
      }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }))
      .mockResolvedValueOnce(new Response(JSON.stringify(artifact), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }));
    vi.stubGlobal("fetch", fetchMock);

    await expect(listArtifacts("platform.team", "backend_api", { limit: 100 })).resolves.toMatchObject({
      items: [{ digest: artifactDigest }],
    });
    await expect(listTags("platform.team", "backend_api", { limit: 100 })).resolves.toMatchObject({
      items: [{ name: "latest", digest: artifactDigest }],
    });
    await expect(getArtifact("platform.team", "backend_api", artifactDigest)).resolves.toMatchObject({
      digest: artifactDigest,
    });

    expect(fetchMock).toHaveBeenNthCalledWith(
      1,
      "/api/v1/namespaces/platform.team/repositories/backend_api/artifacts?limit=100",
      expect.objectContaining({ credentials: "include" }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      "/api/v1/namespaces/platform.team/repositories/backend_api/tags?limit=100",
      expect.objectContaining({ credentials: "include" }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      3,
      `/api/v1/namespaces/platform.team/repositories/backend_api/artifacts/${encodeURIComponent(artifactDigest)}`,
      expect.objectContaining({ credentials: "include" }),
    );
  });
});
