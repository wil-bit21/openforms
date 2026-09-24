import { diffLines } from "diff";

export type DiffCell = { n: number; text: string };
export type DiffRow = { kind: "same" | "removed" | "added" | "changed"; left: DiffCell | null; right: DiffCell | null };

function lines(value: string): string[] {
  const out = value.split("\n");
  if (out[out.length - 1] === "") out.pop();
  return out;
}

export function sideBySide(a: string, b: string): DiffRow[] {
  const parts = diffLines(a, b);
  const rows: DiffRow[] = [];
  let leftN = 1;
  let rightN = 1;
  for (let i = 0; i < parts.length; i++) {
    const part = parts[i];
    if (!part.added && !part.removed) {
      for (const text of lines(part.value)) rows.push({ kind: "same", left: { n: leftN++, text }, right: { n: rightN++, text } });
      continue;
    }
    if (part.removed && parts[i + 1]?.added) {
      const left = lines(part.value);
      const right = lines(parts[i + 1].value);
      const size = Math.max(left.length, right.length);
      for (let k = 0; k < size; k++) {
        rows.push({
          kind: "changed",
          left: k < left.length ? { n: leftN++, text: left[k] } : null,
          right: k < right.length ? { n: rightN++, text: right[k] } : null,
        });
      }
      i++;
      continue;
    }
    if (part.removed) {
      for (const text of lines(part.value)) rows.push({ kind: "removed", left: { n: leftN++, text }, right: null });
    } else {
      for (const text of lines(part.value)) rows.push({ kind: "added", left: null, right: { n: rightN++, text } });
    }
  }
  return rows;
}
