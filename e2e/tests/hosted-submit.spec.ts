import { expect, test } from "@playwright/test";

test("respondent submits the hosted form and tracks its status", async ({ page }) => {
  await page.goto("/f/e2e-apply");
  await expect(page.getByRole("heading", { name: "E2E application" })).toBeVisible();

  await page.getByLabel(/^Name/).fill("Hosted Tester");
  await page.getByLabel(/^Email/).fill(`hosted+${Date.now()}@example.com`);
  await page.getByRole("button", { name: "Send" }).click();

  await expect(page.getByText("Thanks, e2e!")).toBeVisible();
  await page.getByRole("link", { name: "Track your submission" }).click();

  await expect(page).toHaveURL(/\/s\/[0-9a-f-]{36}\?token=/);
  await expect(page.getByText("New").first()).toBeVisible();
});

test("validation errors keep the respondent on the form", async ({ page }) => {
  await page.goto("/f/e2e-apply");
  await page.getByLabel(/^Name/).fill("No Email");
  await page.getByLabel(/^Email/).fill("not-an-email");
  await page.getByRole("button", { name: "Send" }).click();
  await expect(page.getByLabel(/^Email/)).toHaveAttribute("aria-invalid", "true");
  await expect(page.getByText("Thanks, e2e!")).toHaveCount(0);
});

test("unknown forms show a friendly page", async ({ page }) => {
  await page.goto("/f/does-not-exist");
  await expect(page.getByText("This form isn't available")).toBeVisible();
});
