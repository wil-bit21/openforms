import type { Problem } from "./types";

export function dedupeProblems(problems: Problem[]): Problem[] {
  const seen = new Set<string>();
  return problems.filter((p) => {
    const key = `${p.path}\u0000${p.message}`;
    if (seen.has(key)) return false;
    seen.add(key);
    return true;
  });
}

export function mergeProblems(...lists: Problem[][]): Problem[] {
  return dedupeProblems(lists.flat());
}

export function problemsAt(problems: Problem[], path: string): Problem[] {
  return problems.filter((p) => p.path === path);
}

export function problemsUnder(problems: Problem[], prefix: string): Problem[] {
  return problems.filter(
    (p) => p.path === prefix || p.path.startsWith(`${prefix}.`) || p.path.startsWith(`${prefix}[`),
  );
}

export function indexFromPath(path: string, collection: string): number | null {
  const match = new RegExp(`^${collection}\\[(\\d+)\\]`).exec(path);
  return match ? Number(match[1]) : null;
}
