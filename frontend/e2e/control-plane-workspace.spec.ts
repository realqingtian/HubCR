import { expect, test, type Page, type Route } from "@playwright/test";

const timestamp = "2026-08-01T12:00:00Z";
const ownerID = "11111111-1111-4111-8111-111111111111";
const organizationID = "22222222-2222-4222-8222-222222222222";
const repositoryID = "33333333-3333-4333-8333-333333333333";
const memberID = "44444444-4444-4444-8444-444444444444";
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

function fulfill(route: Route, body: unknown, status = 200) {
  return route.fulfill({ status, headers: corsHeaders, body: JSON.stringify(body) });
}

async function installControlPlaneMock(page: Page, authenticatedInitially: boolean, denyRepositoryCreate = false) {
  let authenticated = authenticatedInitially;
  let organizations: unknown[] = [];
  const repositories = new Map<string, unknown[]>();
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
      if (denyRepositoryCreate) {
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
  await installControlPlaneMock(page, true, true);
  await page.goto("/");

  await page.getByLabel("Name", { exact: true }).fill("denied-app");
  await page.getByRole("button", { name: "Create repository" }).click();
  await expect(page.getByText("Your account does not have permission for this action.", { exact: true })).toBeVisible();
});
