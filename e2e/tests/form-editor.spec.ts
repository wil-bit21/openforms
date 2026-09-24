import { expect, test } from "@playwright/test";
import { api } from "../lib/api";
import { loginAdmin } from "../lib/admin";

type Versions = { items: Array<{ version: number; source: string }> };

test("saving the visual form editor creates a new version", async ({ page }) => {
  const before = await api<Versions>("GET", "/api/v1/forms/contact/versions");
  expect(before.status).toBe(200);

  await loginAdmin(page, "admin@demo.local", "demo1234");
  await page.goto("/admin/forms/contact/edit");

  await page.getByText(/^Your name/).first().click();
  const label = `Your name (${Date.now()})`;
  await page.getByLabel("Label", { exact: true }).fill(label);
  await page.getByRole("button", { name: "Save" }).click();

  await expect
    .poll(async () => (await api<Versions>("GET", "/api/v1/forms/contact/versions")).body.items.length)
    .toBe(before.body.items.length + 1);
  const after = await api<Versions>("GET", "/api/v1/forms/contact/versions");
  expect(after.body.items[0].source).toBe("ui");

  const form = await api<{ form: { definition: { fields: Array<{ key: string; label: string }> } } }>(
    "GET",
    "/api/v1/forms/contact",
  );
  expect(form.body.form.definition.fields.find((f) => f.key === "name")?.label).toBe(label);
});
