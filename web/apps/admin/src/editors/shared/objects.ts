export function applyPatch<T extends object>(
  target: T,
  patch: Partial<T>,
  opts: { dropEmptyStrings?: boolean } = {},
): T {
  const dropEmpty = opts.dropEmptyStrings ?? true;
  const out: Record<string, unknown> = { ...(target as Record<string, unknown>) };
  for (const [key, value] of Object.entries(patch)) {
    if (value === undefined || (dropEmpty && value === "")) delete out[key];
    else out[key] = value;
  }
  return out as T;
}

export function nextNumbered(prefix: string, taken: Iterable<string>): string {
  const set = new Set(taken);
  for (let n = 1; ; n++) {
    const candidate = `${prefix}${n}`;
    if (!set.has(candidate)) return candidate;
  }
}

export function uniqueName(base: string, taken: Iterable<string>): string {
  const set = new Set(taken);
  if (!set.has(base)) return base;
  for (let n = 2; ; n++) {
    const candidate = `${base}${n}`;
    if (!set.has(candidate)) return candidate;
  }
}

export function moveItem<T>(items: T[], from: number, to: number): T[] {
  const next = items.slice();
  const [item] = next.splice(from, 1);
  next.splice(to, 0, item);
  return next;
}

export function clone<T>(value: T): T {
  return structuredClone(value);
}

export function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

export function str(value: unknown): string {
  return typeof value === "string" ? value : "";
}

export function objects<T>(value: unknown): T[] {
  return Array.isArray(value) ? (value.filter(isObject) as T[]) : [];
}

export function unique<T>(items: T[]): T[] {
  return Array.from(new Set(items));
}
