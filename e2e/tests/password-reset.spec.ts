import { expect, test } from "@playwright/test";
import { api } from "../lib/api";
import { loginAdmin } from "../lib/admin";
import { listMessages, messageText } from "../lib/mailpit";

test("a user resets a forgotten password from the emailed link", async ({ page }) => {
  const email = `reset-${Date.now()}@e2e.local`;
  const created = await api("POST", "/api/v1/users", { email, name: "Reset Tester", password: "old-password-1", roles: ["reviewer"] });
  expect(created.status).toBe(201);

  await page.goto("/admin/login");
  await page.getByRole("link", { name: "Forgot your password?" }).click();
  await page.getByLabel("Email").fill(email);
  await page.getByRole("button", { name: "Send reset link" }).click();
  await expect(page.getByRole("status")).toContainText(`If an account exists for ${email}`);

  let id: string | undefined;
  await expect
    .poll(async () => (id = (await listMessages()).find((m) => m.To[0]?.Address === email)?.ID), { timeout: 30_000 })
    .toBeTruthy();
  const link = (await messageText(id!)).match(/https?:\/\/\S+\/admin\/reset-password\?token=\S+/)?.[0];
  expect(link).toBeTruthy();

  await page.goto(new URL(link!).pathname + new URL(link!).search);
  await page.getByLabel("New password", { exact: true }).fill("new-password-1");
  await page.getByLabel("Confirm new password").fill("new-password-1");
  await page.getByRole("button", { name: "Set new password" }).click();
  await expect(page.getByRole("status")).toContainText("Your password was changed");

  await loginAdmin(page, email, "new-password-1");
});
