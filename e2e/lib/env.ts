import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

export const E2E_DIR = join(dirname(fileURLToPath(import.meta.url)), "..");
export const REPO_ROOT = join(E2E_DIR, "..");
export const BASE_URL = process.env.E2E_BASE_URL ?? "http://localhost:8080";
export const MAILPIT_URL = process.env.E2E_MAILPIT_URL ?? "http://localhost:8025";
export const WEBHOOK_SECRET = process.env.E2E_WEBHOOK_SECRET ?? "e2e-secret";
export const SINK_PORT = 9911;

export const E2E_REVIEWER = {
  email: "e2e-reviewer@example.com",
  name: "E2E Reviewer",
  password: "password123",
};

export interface E2EState {
  apiKey: string;
}

const STATE_FILE = join(E2E_DIR, ".state", "state.json");

export function writeState(state: E2EState): void {
  mkdirSync(dirname(STATE_FILE), { recursive: true });
  writeFileSync(STATE_FILE, JSON.stringify(state, null, 2));
}

export function readState(): E2EState {
  return JSON.parse(readFileSync(STATE_FILE, "utf8")) as E2EState;
}
