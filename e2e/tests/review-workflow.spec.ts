import { createHmac } from "node:crypto";
import { expect, test } from "@playwright/test";
import { api, createPublicSubmission } from "../lib/api";
import { loginAdmin } from "../lib/admin";
import { E2E_REVIEWER, WEBHOOK_SECRET } from "../lib/env";
import { listMessages } from "../lib/mailpit";
import { receivedDeliveries, type Delivery } from "../lib/webhook-sink";

test.describe.serial("review workflow", () => {
  const email = `e2e+${Date.now()}@example.com`;
  let sub: { id: string; receiptToken: string };

  test.beforeAll(async () => {
    sub = await createPublicSubmission("e2e-apply", { name: "Workflow Tester", email });
  });

  test("guard rejects approve without the required note", async () => {
    const res = await api<{ error: { code: string; details?: Array<{ path: string }> } }>(
      "POST",
      `/api/v1/submissions/${sub.id}/transitions`,
      { transition: "approve" },
    );
    expect(res.status).toBe(422);
    expect(res.body.error.code).toBe("validation_failed");
    expect(res.body.error.details).toEqual(expect.arrayContaining([expect.objectContaining({ path: "fields.note" })]));
  });

  test("reviewer approves in the admin UI, providing the note", async ({ page }) => {
    await loginAdmin(page, E2E_REVIEWER.email, E2E_REVIEWER.password);
    await page.goto(`/admin/submissions/${sub.id}`);

    await page.getByRole("button", { name: "Approve" }).click();
    const dialog = page.getByRole("dialog");
    await dialog.getByLabel("Note").fill("Great candidate");
    await dialog.getByRole("button", { name: /^(Approve|Confirm)$/ }).click();

    await expect(dialog).toBeHidden();
    await expect(page.getByText("Approved").first()).toBeVisible();
    await expect(page.getByRole("button", { name: "Approve" })).toHaveCount(0);
  });

  test("respondent status page reflects the transition", async ({ page }) => {
    await page.goto(`/s/${sub.id}?token=${encodeURIComponent(sub.receiptToken)}`);
    await expect(page.getByText("Approved").first()).toBeVisible();
    const status = await api<{ state: string; terminal: boolean }>(
      "GET",
      `/api/v1/public/submissions/${sub.id}?token=${encodeURIComponent(sub.receiptToken)}`,
      undefined,
      null,
    );
    expect(status.body).toMatchObject({ state: "approved", terminal: true });
  });

  test("approval email is delivered to Mailpit", async () => {
    await expect
      .poll(
        async () => (await listMessages()).find((m) => m.Subject === `E2E approved ${sub.id}`)?.To[0]?.Address,
        { timeout: 30_000 },
      )
      .toBe(email);
  });

  test("webhook is delivered once, signed with the shared secret", async () => {
    let delivery: Delivery | undefined;
    await expect
      .poll(
        async () => {
          delivery = (await receivedDeliveries()).find(
            (d) =>
              d.headers["x-openforms-event"] === "submission.transitioned" &&
              JSON.parse(d.body).submission?.id === sub.id,
          );
          return Boolean(delivery);
        },
        { timeout: 30_000 },
      )
      .toBe(true);

    const body = JSON.parse(delivery!.body);
    expect(body.transition).toMatchObject({ key: "approve", from: "new", to: "approved" });
    expect(body.form).toMatchObject({ slug: "e2e-apply" });
    const expected = "sha256=" + createHmac("sha256", WEBHOOK_SECRET).update(delivery!.body).digest("hex");
    expect(delivery!.headers["x-openforms-signature"]).toBe(expected);
    expect(delivery!.headers["x-openforms-delivery"]).toMatch(/^\d+$/);
  });

  test("timeline records the transition and successful actions", async () => {
    await expect
      .poll(
        async () => {
          const res = await api<{ events: Array<{ type: string }> }>("GET", `/api/v1/submissions/${sub.id}`);
          return res.body.events.filter((e) => e.type === "action_succeeded").length;
        },
        { timeout: 30_000 },
      )
      .toBe(2);
    const res = await api<{ events: Array<{ type: string; transition: string | null }> }>(
      "GET",
      `/api/v1/submissions/${sub.id}`,
    );
    expect(res.body.events.map((e) => e.type)).toEqual(expect.arrayContaining(["created", "transition"]));
  });
});
