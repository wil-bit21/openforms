import { OpenFormsError } from "@openforms/sdk";

export function errorMessage(err: unknown): string {
  if (err instanceof OpenFormsError) return err.message || err.code;
  if (err instanceof Error) return err.message;
  return "Something went wrong.";
}

export function errorCode(err: unknown): string | undefined {
  return err instanceof OpenFormsError ? err.code : undefined;
}

export function problemsByPath(err: unknown): Record<string, string> {
  if (!(err instanceof OpenFormsError)) return {};
  const out: Record<string, string> = {};
  for (const p of err.details ?? []) out[p.path] = p.message;
  return out;
}
