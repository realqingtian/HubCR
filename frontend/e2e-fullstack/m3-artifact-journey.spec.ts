import { expect, test } from "@playwright/test";

const username = process.env.HUBCR_M3_E2E_USERNAME ?? "m2-e2e-owner";
const password = process.env.HUBCR_M3_E2E_PASSWORD ?? "m2-e2e-password";
const digestPattern = /^sha256:[0-9a-f]{64}$/;

test("completes M3 journey 7 from a real Registry push to Web discovery", async ({ page }) => {
  await page.goto("/");
  await page.getByLabel("Username").fill(username);
  await page.getByLabel("Password").fill(password);
  await page.getByRole("button", { name: "Sign in" }).click();

  await expect(page.getByRole("heading", { name: "Control-plane workspace" })).toBeVisible();
  await page.goto("/namespaces/m2-e2e-team/repositories/private-image");

  await expect(page.getByRole("heading", { name: "m2-e2e-team/private-image" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Quick start" })).toBeVisible();
  await expect(page.getByText("docker login hubcr.io", { exact: true })).toBeVisible();
  await expect(page.getByText("docker pull hubcr.io/m2-e2e-team/private-image:TAG", { exact: true })).toBeVisible();
  await expect(page.getByText(/docker push hubcr.io\/m2-e2e-team\/private-image:TAG/)).toBeVisible();
  await expect(page.getByRole("heading", { name: "Current tags" })).toBeVisible();
  const tagCard = page.locator("article").filter({ hasText: "smoke" });
  await expect(tagCard.getByText("smoke", { exact: true })).toBeVisible();
  const digest = (await tagCard.locator("p.font-mono").textContent())?.trim() ?? "";
  expect(digest).toMatch(digestPattern);

  const artifactColumn = page.getByRole("heading", { name: "Immutable artifacts" }).locator("..");
  await expect(artifactColumn.getByText(digest, { exact: true })).toBeVisible();
  await tagCard.getByRole("link", { name: "View immutable Artifact" }).click();

  await expect(page.getByRole("heading", { name: digest })).toBeVisible();
  await expect(page.getByText("Immutable Artifact", { exact: true })).toBeVisible();
  await expect(page.getByText("Media type").locator("..")).not.toContainText("Unavailable");
  await expect(page.getByText("Size", { exact: true }).locator("..")).not.toContainText("Unavailable");
  await expect(page.getByText(/Scan, signature presence, cryptographic validity/)).toBeVisible();
});
