import { expect, test } from "@playwright/test";
import { listMessages } from "../lib/mailpit";

test("visitor submits on /demo and plays the reviewer", async ({ page }) => {
  const email = `demo+${Date.now()}@example.com`;
  await page.goto("/demo");

  const collect = page.getByRole("region", { name: /Collect/ });
  await collect.getByLabel(/Full name/).fill("Demo Visitor");
  await collect.getByLabel(/^Email/).fill(email);
  await collect.getByLabel(/^Role/).selectOption("engineer");
  await collect.getByLabel(/Years of experience/).fill("5");
  await collect.getByLabel(/I agree/).check();
  await collect.getByRole("button", { name: "Send application" }).click();

  const track = page.getByRole("region", { name: /Track & review/ });
  await expect(track.getByRole("link", { name: "Open in the admin inbox" })).toHaveAttribute(
    "href",
    /\/admin\/submissions\/[0-9a-f-]{36}$/,
  );

  await track.getByRole("button", { name: "Start screening" }).click();
  // Now in "screening": the reviewer panel offers the next transition, which needs a score.
  const invite = track.getByRole("button", { name: "Invite to interview" });
  await expect(invite).toBeDisabled();
  await track.getByLabel("Score").fill("4");
  await invite.click();

  // In "interview", only the hiring manager may hire.
  await expect(track.getByText("Not allowed for Reviewer")).toBeVisible();
  await track.getByRole("button", { name: "Hiring manager" }).click();
  await track.getByRole("button", { name: "Hire" }).click();
  await expect(track.getByText(/reached a final state: Hired/)).toBeVisible();

  await expect
    .poll(async () => (await listMessages()).filter((m) => m.To[0]?.Address === email).map((m) => m.Subject).sort(), {
      timeout: 30_000,
    })
    .toEqual(["Let's talk — interview invitation", "We got your application", "Welcome aboard!"]);
});

test("demo embed renders the contact form", async ({ page }) => {
  await page.goto("/demo");
  const frame = page.frameLocator('iframe[src*="/f/contact"]');
  await expect(frame.getByLabel(/Your name/)).toBeVisible();
});
