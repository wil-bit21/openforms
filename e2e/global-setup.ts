import { execFileSync } from "node:child_process";
import { api } from "./lib/api";
import { BASE_URL, E2E_REVIEWER, REPO_ROOT, writeState } from "./lib/env";
import { clearMessages } from "./lib/mailpit";
import { startWebhookSink } from "./lib/webhook-sink";
import { E2E_FORM, E2E_WORKFLOW } from "./fixtures/bundle";

async function waitForHealthy(url: string, timeoutMs: number): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  let last = "";
  while (Date.now() < deadline) {
    try {
      const res = await fetch(url);
      if (res.ok) return;
      last = `HTTP ${res.status}`;
    } catch (e) {
      last = String(e);
    }
    await new Promise((r) => setTimeout(r, 1000));
  }
  throw new Error(`openforms not healthy at ${url} after ${timeoutMs}ms (${last}). Run: docker compose --profile app up -d --build`);
}

function createApiKey(): string {
  const out = execFileSync(
    "docker",
    ["compose", "--profile", "app", "exec", "-T", "openforms", "openforms", "admin", "create-api-key", "--name", `e2e-${Date.now()}`, "--roles", "admin"],
    { cwd: REPO_ROOT, encoding: "utf8" },
  );
  const match = out.match(/^ofk_[A-Za-z0-9]+$/m);
  if (!match) throw new Error(`could not find an API key in output:\n${out}`);
  return match[0];
}

export default async function globalSetup() {
  await waitForHealthy(`${BASE_URL}/healthz`, 180_000);
  const sink = await startWebhookSink();

  const apiKey = process.env.OPENFORMS_E2E_API_KEY ?? createApiKey();
  writeState({ apiKey });

  const applied = await api("POST", "/api/v1/definitions/apply?source=api", {
    forms: [E2E_FORM],
    workflows: [E2E_WORKFLOW],
  });
  if (applied.status !== 200) throw new Error(`apply e2e bundle: ${applied.status} ${JSON.stringify(applied.body)}`);

  const user = await api("POST", "/api/v1/users", { ...E2E_REVIEWER, roles: ["reviewer"] });
  if (![200, 201, 409].includes(user.status)) throw new Error(`create reviewer: ${user.status} ${JSON.stringify(user.body)}`);

  await clearMessages();

  return async () => {
    await sink.close();
  };
}
