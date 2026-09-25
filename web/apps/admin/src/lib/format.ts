export type FieldLike = {
  key: string;
  type: string;
  label?: string;
  options?: { value: string; label: string }[];
};

export function relativeTime(iso: string, now: Date = new Date()): string {
  const diff = Math.round((new Date(iso).getTime() - now.getTime()) / 1000);
  const abs = Math.abs(diff);
  const rtf = new Intl.RelativeTimeFormat("en", { numeric: "auto" });
  if (abs < 60) return rtf.format(diff, "second");
  if (abs < 3600) return rtf.format(Math.round(diff / 60), "minute");
  if (abs < 86400) return rtf.format(Math.round(diff / 3600), "hour");
  if (abs < 86400 * 30) return rtf.format(Math.round(diff / 86400), "day");
  return new Date(iso).toLocaleDateString("en-US", { year: "numeric", month: "short", day: "numeric", timeZone: "UTC" });
}

function isEmptyValue(value: unknown): boolean {
  return value === undefined || value === null || value === "" || (Array.isArray(value) && value.length === 0);
}

export function formatValue(value: unknown, field?: FieldLike): string {
  if (isEmptyValue(value)) return "—";
  const label = (v: unknown) => {
    const option = field?.options?.find((o) => o.value === v);
    return option ? option.label : String(v);
  };
  if (typeof value === "boolean") return value ? "Yes" : "No";
  if (Array.isArray(value)) return value.map(label).join(", ");
  if (typeof value === "object") return JSON.stringify(value);
  return label(value);
}

export function submissionSummary(data: Record<string, unknown>, form?: { fields: FieldLike[] }): string {
  const keys = form
    ? form.fields.filter((f) => f.type === "text" || f.type === "email").map((f) => f.key)
    : Object.keys(data);
  const parts: string[] = [];
  for (const key of keys) {
    const v = data[key];
    if (typeof v === "string" && v.trim() !== "") parts.push(v.trim());
    if (parts.length === 2) break;
  }
  return parts.length ? parts.join(" · ") : "—";
}

export function shortId(id: string): string {
  return id.slice(0, 8);
}
