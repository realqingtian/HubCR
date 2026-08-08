import { expect, test, type Page, type Route } from "@playwright/test";

const timestamp = "2026-08-01T12:00:00Z";
const ownerID = "11111111-1111-4111-8111-111111111111";
const organizationID = "22222222-2222-4222-8222-222222222222";
const repositoryID = "33333333-3333-4333-8333-333333333333";
const memberID = "44444444-4444-4444-8444-444444444444";
const manifestDigest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";
const unknownIndexDigest = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb";
const emptyIndexDigest = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc";
const missingDigest = "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd";
const signatureDigest = "sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee";
const keyFingerprint = "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff";
const corsHeaders = {
  "Access-Control-Allow-Credentials": "true",
  "Access-Control-Allow-Headers": "Content-Type",
  "Access-Control-Allow-Methods": "GET, POST, PATCH, OPTIONS",
  "Access-Control-Allow-Origin": "http://127.0.0.1:3000",
  "Content-Type": "application/json",
};

const user = {
  id: ownerID,
  username: "owner",
  personal_namespace: "owner",
  created_at: timestamp,
};

const repositoryFixture = {
  id: repositoryID,
  namespace: "owner",
  name: "backend",
  visibility: "PUBLIC",
  description: "personal images",
  created_by_user_id: ownerID,
  visibility_updated_by_user_id: ownerID,
  visibility_updated_at: timestamp,
  created_at: timestamp,
  updated_at: timestamp,
};

const manifestArtifact = {
  digest: manifestDigest,
  kind: "MANIFEST",
  media_type: "application/vnd.oci.image.manifest.v1+json",
  size_bytes: 512,
  descriptors_complete: false,
  discovered_at: timestamp,
  updated_at: timestamp,
};
const unknownIndexArtifact = {
  digest: unknownIndexDigest,
  kind: "INDEX",
  media_type: "application/vnd.oci.image.index.v1+json",
  descriptors_complete: false,
  discovered_at: timestamp,
  updated_at: timestamp,
};
const emptyIndexArtifact = {
  digest: emptyIndexDigest,
  kind: "INDEX",
  media_type: "application/vnd.oci.image.index.v1+json",
  descriptors_complete: true,
  discovered_at: timestamp,
  updated_at: timestamp,
  manifests: [],
};

function fulfill(route: Route, body: unknown, status = 200) {
  return route.fulfill({ status, headers: corsHeaders, body: JSON.stringify(body) });
}

type ArtifactMode = "success" | "empty" | "forbidden" | "failure";

async function installControlPlaneMock(
  page: Page,
  authenticatedInitially: boolean,
  options: Readonly<{
    artifactMode?: ArtifactMode;
    denyRepositoryCreate?: boolean;
    repositoryCapabilities?: { can_pull: boolean; can_push: boolean };
    securityResponse?: unknown;
    seedRepository?: boolean;
    seedVisibility?: "PUBLIC" | "PRIVATE";
  }> = {},
) {
  let authenticated = authenticatedInitially;
  let organizations: unknown[] = [];
  const repositories = new Map<string, unknown[]>(
    options.seedRepository ? [["owner", [{
      ...repositoryFixture,
      visibility: options.seedVisibility ?? repositoryFixture.visibility,
    }]]] : [],
  );
  const members = new Map<string, unknown[]>();

  await page.route("**/api/v1/**", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname;
    if (request.method() === "OPTIONS") {
      await route.fulfill({ status: 204, headers: corsHeaders });
      return;
    }
    if (path === "/api/v1/auth/me") {
      if (!authenticated) {
        await fulfill(route, {
          error: { code: "authentication_failed", message: "authentication failed" },
          request_id: "e2e-auth",
        }, 401);
        return;
      }
      await fulfill(route, user);
      return;
    }
    if (path === "/api/v1/auth/login" && request.method() === "POST") {
      authenticated = true;
      await fulfill(route, { user, expires_at: "2026-08-02T12:00:00Z" });
      return;
    }
    if (path === "/api/v1/organizations" && request.method() === "GET") {
      await fulfill(route, { items: organizations, meta: { limit: 100 } });
      return;
    }
    if (path === "/api/v1/organizations" && request.method() === "POST") {
      const input = request.postDataJSON() as { name: string; description: string };
      const organization = {
        id: organizationID,
        namespace: input.name.toLowerCase(),
        description: input.description,
        created_by_user_id: ownerID,
        created_at: timestamp,
        updated_at: timestamp,
      };
      organizations = [organization];
      await fulfill(route, organization, 201);
      return;
    }
    const memberMatch = path.match(/^\/api\/v1\/organizations\/([^/]+)\/members$/);
    if (memberMatch && request.method() === "GET") {
      await fulfill(route, { items: members.get(memberMatch[1]) ?? [], meta: { limit: 100 } });
      return;
    }
    if (memberMatch && request.method() === "POST") {
      const input = request.postDataJSON() as { user_id: string; role: string };
      members.set(memberMatch[1], [{
        user_id: input.user_id,
        role: input.role,
        added_by_user_id: ownerID,
        created_at: timestamp,
        updated_at: timestamp,
      }]);
      await route.fulfill({ status: 204, headers: corsHeaders });
      return;
    }
    const repositoryMatch = path.match(/^\/api\/v1\/namespaces\/([^/]+)\/repositories$/);
    if (repositoryMatch && request.method() === "GET") {
      await fulfill(route, { items: repositories.get(repositoryMatch[1]) ?? [], meta: { limit: 100 } });
      return;
    }
    if (repositoryMatch && request.method() === "POST") {
      if (options.denyRepositoryCreate) {
        await fulfill(route, {
          error: { code: "forbidden", message: "repository action is forbidden" },
          request_id: "e2e-forbidden",
        }, 403);
        return;
      }
      const namespace = repositoryMatch[1];
      const input = request.postDataJSON() as { name: string; visibility: string; description: string };
      const repository = {
        id: repositoryID,
        namespace,
        name: input.name.toLowerCase(),
        visibility: input.visibility,
        description: input.description,
        created_by_user_id: ownerID,
        visibility_updated_by_user_id: ownerID,
        visibility_updated_at: timestamp,
        created_at: timestamp,
        updated_at: timestamp,
      };
      repositories.set(namespace, [repository]);
      await fulfill(route, repository, 201);
      return;
    }
    const repositoryDetailMatch = path.match(/^\/api\/v1\/namespaces\/([^/]+)\/repositories\/([^/]+)$/);
    if (repositoryDetailMatch && request.method() === "GET") {
      const candidates = repositories.get(repositoryDetailMatch[1]) ?? [];
      const repository = candidates.find((candidate) => (
        typeof candidate === "object" && candidate !== null &&
        "name" in candidate && candidate.name === repositoryDetailMatch[2]
      ));
      if (repository !== undefined) {
        await fulfill(route, {
          ...repository,
          capabilities: options.repositoryCapabilities ?? { can_pull: true, can_push: true },
        });
        return;
      }
      await fulfill(route, {
        error: { code: "not_found", message: "resource not found" },
        request_id: "e2e-repository-not-found",
      }, 404);
      return;
    }
    const artifactListMatch = path.match(/^\/api\/v1\/namespaces\/([^/]+)\/repositories\/([^/]+)\/artifacts$/);
    const tagListMatch = path.match(/^\/api\/v1\/namespaces\/([^/]+)\/repositories\/([^/]+)\/tags$/);
    const artifactSecurityMatch = path.match(/^\/api\/v1\/namespaces\/([^/]+)\/repositories\/([^/]+)\/artifacts\/([^/]+)\/security$/);
    const artifactDetailMatch = path.match(/^\/api\/v1\/namespaces\/([^/]+)\/repositories\/([^/]+)\/artifacts\/([^/]+)$/);
    if ((artifactListMatch || tagListMatch || artifactSecurityMatch || artifactDetailMatch) && request.method() === "GET") {
      const mode = options.artifactMode ?? "success";
      if (mode === "forbidden") {
        await fulfill(route, {
          error: { code: "forbidden", message: "repository action is forbidden" },
          request_id: "e2e-artifact-forbidden",
        }, 403);
        return;
      }
      if (mode === "failure") {
        await fulfill(route, {
          error: { code: "internal_error", message: "artifact service failed" },
          request_id: "e2e-artifact-failure",
        }, 503);
        return;
      }
      if (artifactListMatch) {
        await fulfill(route, {
          items: mode === "empty" ? [] : [manifestArtifact, unknownIndexArtifact, emptyIndexArtifact],
          meta: { limit: 100 },
        });
        return;
      }
      if (tagListMatch) {
        await fulfill(route, {
          items: mode === "empty" ? [] : [{
            name: "latest",
            digest: manifestDigest,
            created_at: timestamp,
            updated_at: timestamp,
          }],
          meta: { limit: 100 },
        });
        return;
      }
      if (artifactSecurityMatch) {
        const digest = decodeURIComponent(artifactSecurityMatch[3] ?? "");
        await fulfill(route, options.securityResponse ?? {
          digest,
          scan: { state: "QUEUED", attempts: 0, updated_at: timestamp },
          sbom: { state: "QUEUED", attempts: 0, updated_at: timestamp },
          signature: { state: "ABSENT", evidence: [] },
        });
        return;
      }
      const digest = decodeURIComponent(artifactDetailMatch?.[3] ?? "");
      const artifact = [manifestArtifact, unknownIndexArtifact, emptyIndexArtifact]
        .find((candidate) => candidate.digest === digest);
      if (artifact) {
        await fulfill(route, artifact);
        return;
      }
      await fulfill(route, {
        error: { code: "not_found", message: "resource not found" },
        request_id: "e2e-artifact-not-found",
      }, 404);
      return;
    }
    await fulfill(route, {
      error: { code: "not_found", message: "mock route not found" },
      request_id: "e2e-not-found",
    }, 404);
  });
}

test("signs in and completes the personal and organization ownership flows", async ({ page }) => {
  await installControlPlaneMock(page, false);
  await page.goto("/");

  await expect(page.getByRole("heading", { name: "Registry ownership starts with a clear namespace." })).toBeVisible();
  await page.getByLabel("Username").fill("owner");
  await page.getByLabel("Password").fill("correct password");
  await page.getByRole("button", { name: "Sign in" }).click();

  await expect(page.getByRole("heading", { name: "Control-plane workspace" })).toBeVisible();
  await expect(page.getByText("hubcr.io/owner", { exact: true }).first()).toBeVisible();
  await expect(page.getByText("No repositories yet")).toBeVisible();

  await page.getByLabel("Name", { exact: true }).fill("backend");
  await page.getByLabel("Description").first().fill("personal images");
  await page.getByRole("button", { name: "Create repository" }).click();
  await expect(page.getByText("owner/backend")).toBeVisible();

  await page.getByRole("link", { name: "View namespace" }).click();
  await expect(page).toHaveURL(/\/namespaces\/owner$/);
  await expect(page.getByRole("heading", { name: "hubcr.io/owner" })).toBeVisible();
  await page.getByRole("link", { name: "View repository" }).click();
  await expect(page).toHaveURL(/\/namespaces\/owner\/repositories\/backend$/);
  await expect(page.getByRole("heading", { name: "owner/backend" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Quick start" })).toBeVisible();
  await expect(page.getByText("docker login hubcr.io", { exact: true })).toBeVisible();
  await expect(page.getByText("docker pull hubcr.io/owner/backend:TAG", { exact: true })).toBeVisible();
  await expect(page.getByText(/docker push hubcr.io\/owner\/backend:TAG/)).toBeVisible();
  await expect(page.getByRole("heading", { name: "Artifacts and tags" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Current tags" })).toBeVisible();
  await expect(page.getByText("latest", { exact: true })).toBeVisible();
  await expect(page.getByText(manifestDigest, { exact: true }).first()).toBeVisible();
  await page.getByRole("link", { name: "View immutable Artifact" }).click();
  await expect(page).toHaveURL(new RegExp(`/artifacts/sha256(?:%3A|:)${"a".repeat(64)}$`, "i"));
  await expect(page.getByRole("heading", { name: manifestDigest })).toBeVisible();
  await expect(page.getByText("Manifest Artifact", { exact: true })).toBeVisible();
  await expect(page.getByText("Verification not configured", { exact: true })).toBeVisible();
  await page.getByLabel("Breadcrumb").getByRole("link", { name: "backend" }).click();
  await page.getByLabel("Breadcrumb").getByRole("link", { name: "Overview" }).click();
  await expect(page.getByRole("heading", { name: "Control-plane workspace" })).toBeVisible();

  await page.getByLabel("Namespace", { exact: true }).fill("platform-team");
  await page.getByLabel("Description").nth(1).fill("platform team");
  await page.getByRole("button", { name: "Create organization" }).click();
  await expect(page.getByRole("heading", { name: "Members" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Organization repositories" })).toBeVisible();

  await page.getByLabel("User ID").fill(memberID);
  await page.getByLabel("Role").selectOption("READER");
  await page.getByRole("button", { name: "Add member" }).click();
  await expect(page.getByText(memberID)).toBeVisible();
});

test("shows an actionable server-failure state", async ({ page }) => {
  await page.route("**/api/v1/**", async (route) => {
    await fulfill(route, {
      error: { code: "internal_error", message: "service unavailable" },
      request_id: "e2e-failure",
    }, 503);
  });
  await page.goto("/");

  await expect(page.getByText("Control plane unavailable")).toBeVisible();
  await expect(page.getByRole("button", { name: "Try again" })).toBeVisible();
});

test("shows backend authorization denial instead of inferring permission", async ({ page }) => {
  await installControlPlaneMock(page, true, { denyRepositoryCreate: true });
  await page.goto("/");

  await page.getByLabel("Name", { exact: true }).fill("denied-app");
  await page.getByRole("button", { name: "Create repository" }).click();
  await expect(page.getByText("Your account does not have permission for this action.", { exact: true })).toBeVisible();
});

test("shows truthful empty Artifact and Tag discovery states", async ({ page }) => {
  await installControlPlaneMock(page, true, { artifactMode: "empty", seedRepository: true });
  await page.goto("/namespaces/owner/repositories/backend");

  await expect(page.getByText("No tags", { exact: true })).toBeVisible();
  await expect(page.getByText("No artifacts", { exact: true })).toBeVisible();
});

test("shows public Pull but withholds Push commands from a read-only caller", async ({ page }) => {
  await installControlPlaneMock(page, true, {
    repositoryCapabilities: { can_pull: true, can_push: false },
    seedRepository: true,
  });
  await page.goto("/namespaces/owner/repositories/backend");

  await expect(page.getByText("This public pull does not require Registry login.")).toBeVisible();
  await expect(page.getByText("docker pull hubcr.io/owner/backend:TAG", { exact: true })).toBeVisible();
  await expect(page.getByText("Push access unavailable", { exact: true })).toBeVisible();
  await expect(page.getByText("docker login hubcr.io", { exact: true })).toHaveCount(0);
  await expect(page.getByText(/docker push hubcr.io\/owner\/backend:TAG/)).toHaveCount(0);
});

test("requires Registry login for a private read-only Repository", async ({ page }) => {
  await installControlPlaneMock(page, true, {
    repositoryCapabilities: { can_pull: true, can_push: false },
    seedRepository: true,
    seedVisibility: "PRIVATE",
  });
  await page.goto("/namespaces/owner/repositories/backend");

  await expect(page.getByText("docker login hubcr.io", { exact: true })).toBeVisible();
  await expect(page.getByText("docker pull hubcr.io/owner/backend:TAG", { exact: true })).toBeVisible();
  await expect(page.getByText("Push access unavailable", { exact: true })).toBeVisible();
});

test("separates Artifact authorization denial from service failure", async ({ page }) => {
  await installControlPlaneMock(page, true, { artifactMode: "forbidden", seedRepository: true });
  await page.goto("/namespaces/owner/repositories/backend");

  await expect(page.getByText("Current tags access denied", { exact: true })).toBeVisible();
  await expect(page.getByText("Immutable artifacts access denied", { exact: true })).toBeVisible();
  await expect(page.getByText("Your account does not have permission for this action.").first()).toBeVisible();
});

test("distinguishes unknown and confirmed-empty Index descriptor sets", async ({ page }) => {
  await installControlPlaneMock(page, true);

  await page.goto(`/namespaces/owner/repositories/backend/artifacts/${unknownIndexDigest}`);
  await expect(page.getByText("Descriptor set unavailable", { exact: true })).toBeVisible();

  await page.goto(`/namespaces/owner/repositories/backend/artifacts/${emptyIndexDigest}`);
  await expect(page.getByText("Confirmed empty descriptor set", { exact: true })).toBeVisible();
});

test("keeps Artifact non-disclosure truthful on a direct route", async ({ page }) => {
  await installControlPlaneMock(page, true);
  await page.goto(`/namespaces/owner/repositories/backend/artifacts/${missingDigest}`);

  await expect(page.getByText("Artifact not found", { exact: true })).toBeVisible();
  await expect(page.getByText("The Digest does not exist in this repository or your session cannot discover it.")).toBeVisible();
});

test("rejects an invalid Artifact Digest route before making an API request", async ({ page }) => {
  await installControlPlaneMock(page, true);
  await page.goto("/namespaces/owner/repositories/backend/artifacts/not-a-digest");

  await expect(page.getByRole("heading", { name: "Artifact route not found" })).toBeVisible();
  await expect(page.getByText(/sha256:/)).toBeVisible();
});

test("keeps repository non-disclosure truthful on a direct route", async ({ page }) => {
  await installControlPlaneMock(page, true);
  await page.goto("/namespaces/owner/repositories/missing");

  await expect(page.getByText("Repository not found")).toBeVisible();
  await expect(page.getByText("The repository does not exist or your current session cannot discover it.")).toBeVisible();
});

test("keeps authenticated navigation usable at a mobile width", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await installControlPlaneMock(page, true);
  await page.goto("/namespaces/owner");

  await expect(page.getByLabel("Workspace").getByRole("link", { name: "Overview" })).toBeVisible();
  await expect(page.getByLabel("Workspace").getByRole("link", { name: "Personal namespace" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Sign out" })).toBeVisible();
  const horizontalOverflow = await page.evaluate(() => document.documentElement.scrollWidth > document.documentElement.clientWidth);
  expect(horizontalOverflow).toBe(false);
});

test("renders digest-bound security trust states without mobile overflow", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await installControlPlaneMock(page, true, {
    securityResponse: {
      digest: manifestDigest,
      scan: {
        state: "COMPLETED", attempts: 1, updated_at: timestamp, completed_at: timestamp,
        tool: {
          name: "TRIVY", scanner_version: "0.72.0", database_schema_version: 2,
          database_updated_at: timestamp, database_downloaded_at: timestamp,
        },
        finding_count: 2,
        severity_counts: { HIGH: 1, MEDIUM: 1 },
      },
      sbom: {
        state: "COMPLETED", attempts: 1, updated_at: timestamp, completed_at: timestamp,
        format: "CYCLONEDX_JSON",
      },
      signature: {
        state: "COMPLETED", attempts: 1, updated_at: timestamp,
        policy_id: "55555555-5555-4555-8555-555555555555", policy_version: 2,
        cosign_version: "v3.0.6", completed_at: timestamp,
        evidence: [
          {
            kind: "SIGNATURE", signature_digest: signatureDigest,
            signer_type: "PUBLIC_KEY", key_fingerprint: keyFingerprint,
            cryptographic_state: "VALID", trust_state: "TRUSTED", reason: "TRUSTED_PUBLIC_KEY",
          },
          {
            kind: "ATTESTATION", signature_digest: missingDigest,
            signer_type: "KEYLESS", oidc_issuer: "https://issuer.example", subject: "release@example.com",
            cryptographic_state: "VALID", trust_state: "UNTRUSTED", reason: "VALID_UNTRUSTED_SIGNER",
          },
          {
            kind: "SIGNATURE", signature_digest: unknownIndexDigest,
            signer_type: "PUBLIC_KEY", key_fingerprint: keyFingerprint,
            cryptographic_state: "INVALID", trust_state: "NOT_EVALUATED", reason: "SIGNATURE_INVALID",
          },
        ],
      },
    },
  });
  await page.goto(`/namespaces/owner/repositories/backend/artifacts/${manifestDigest}`);

  await expect(page.getByRole("heading", { name: "Supply-chain security" })).toBeVisible();
  await expect(page.getByText("Scan completed", { exact: true })).toBeVisible();
  await expect(page.getByText("SBOM completed", { exact: true })).toBeVisible();
  await expect(page.getByText("Trusted", { exact: true })).toBeVisible();
  await expect(page.getByText("Valid, untrusted", { exact: true })).toBeVisible();
  await expect(page.getByText("Invalid", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Refresh" }).focus();
  await page.keyboard.press("Enter");
  await expect(page.getByText("Verification completed", { exact: true })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth > document.documentElement.clientWidth)).toBe(false);
});
