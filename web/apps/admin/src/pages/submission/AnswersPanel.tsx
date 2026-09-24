import type { FormDefinition } from "@openforms/sdk";
import { formatValue } from "../../lib/format";

export function AnswersPanel({ form, data }: { form: FormDefinition; data: Record<string, unknown> }) {
  const known = new Set(form.fields.map((f) => f.key));
  const rows = [
    ...form.fields.filter((f) => f.key in data).map((f) => ({ key: f.key, label: f.label, value: formatValue(data[f.key], f) })),
    ...Object.keys(data).filter((k) => !known.has(k)).map((k) => ({ key: k, label: k, value: formatValue(data[k]) })),
  ];
  if (rows.length === 0) return <p className="muted">No answers.</p>;
  return (
    <dl className="answers">
      {rows.map((r) => (
        <div key={r.key} className="answer">
          <dt>{r.label}</dt>
          <dd>{r.value}</dd>
        </div>
      ))}
    </dl>
  );
}
