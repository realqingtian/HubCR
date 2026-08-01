import { expect, test } from "@playwright/test";

const memberID = "92929292-9292-4929-8929-929292929292";

test("completes M1 journeys 1 through 3 against the real control plane", async ({ page }) => {
  await page.goto("/");
  await page.getByLabel("Username").fill("m1-e2e-owner");
  await page.getByLabel("Password").fill("m1-e2e-password");
  await page.getByRole("button", { name: "Sign in" }).click();

  await expect(page.getByRole("heading", { name: "Control-plane workspace" })).toBeVisible();
  await expect(page.getByText("hubcr.io/m1-e2e-owner", { exact: true }).first()).toBeVisible();

  await page.getByLabel("Name", { exact: true }).fill("fullstack-app");
  await page.getByLabel("Description").first().fill("real API repository");
  await page.getByRole("button", { name: "Create repository" }).click();
  const personalRepository = page.locator("article").filter({ hasText: "m1-e2e-owner/fullstack-app" });
  await expect(personalRepository.getByText("PRIVATE", { exact: true })).toBeVisible();
  await personalRepository.getByRole("button", { name: "Make public" }).click();
  await expect(personalRepository.getByText("PUBLIC", { exact: true })).toBeVisible();

  await page.getByLabel("Namespace", { exact: true }).fill("m1-e2e-team");
  await page.getByLabel("Description").nth(1).fill("real API organization");
  await page.getByRole("button", { name: "Create organization" }).click();
  await expect(page.getByRole("heading", { name: "Members" })).toBeVisible();

  await page.getByLabel("User ID").fill(memberID);
  await page.getByLabel("Role").selectOption("READER");
  await page.getByRole("button", { name: "Add member" }).click();
  await expect(page.getByText(memberID)).toBeVisible();
});
