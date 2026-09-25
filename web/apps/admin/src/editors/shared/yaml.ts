import { parse, stringify } from "yaml";

export type ParseResult<T> = { ok: true; value: T } | { ok: false; error: string };

const NOT_A_MAPPING = "The document must be a mapping (key: value pairs) at the top level.";

export function toYaml(value: unknown): string {
  // lineWidth 0 disables folding so long strings stay on one line and diffs stay line-stable.
  return stringify(value, { lineWidth: 0 });
}

export function parseYaml<T = Record<string, unknown>>(text: string): ParseResult<T> {
  let value: unknown;
  try {
    value = parse(text);
  } catch (err) {
    return { ok: false, error: err instanceof Error ? err.message : String(err) };
  }
  if (value === null || typeof value !== "object" || Array.isArray(value)) {
    return { ok: false, error: NOT_A_MAPPING };
  }
  return { ok: true, value: value as T };
}
