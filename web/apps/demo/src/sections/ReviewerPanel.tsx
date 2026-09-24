import { useCallback, useEffect, useState } from "react";
import { OpenFormsError, type AvailableTransition, type SubmissionDetail } from "@openforms/sdk";
import { client } from "../client";

export const DEMO_PASSWORD = "demo1234";

export const PERSONAS = [
  { email: "reviewer@demo.local", label: "Reviewer" },
  { email: "manager@demo.local", label: "Hiring manager" },
] as const;

type Persona = (typeof PERSONAS)[number];

function messageOf(e: unknown): string {
  return e instanceof OpenFormsError ? e.message : "Something went wrong. Please try again.";
}

function isEmpty(v: unknown): boolean {
  return v === undefined || v === null || v === "";
}

export function ReviewerPanel({
  submissionId,
  demoMode,
  onTransitioned,
}: {
  submissionId: string;
  demoMode: boolean;
  onTransitioned: () => void;
}) {
  const [persona, setPersona] = useState<Persona>(PERSONAS[0]);
  const [detail, setDetail] = useState<SubmissionDetail | null>(null);
  const [values, setValues] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    setError(null);
    try {
      await client.login(persona.email, DEMO_PASSWORD);
      setDetail(await client.getSubmission(submissionId));
    } catch (e) {
      setError(messageOf(e));
    }
  }, [persona, submissionId]);

  useEffect(() => {
    if (demoMode) void load();
  }, [demoMode, load]);

  if (!demoMode) {
    return (
      <aside className="panel" aria-labelledby="reviewer-title">
        <h3 id="reviewer-title">Play the reviewer</h3>
        <p className="muted">
          Reviewer simulation is only available when the server runs in demo mode (<code>OPENFORMS_DEMO=true</code>).
          Sign in to the admin inbox to review this submission.
        </p>
      </aside>
    );
  }

  const wfField = (key: string) => detail?.workflow?.fields?.find((f) => f.key === key);

  const missing = (t: AvailableTransition) =>
    t.requireFields.filter((k) => isEmpty(detail?.submission.fields[k]) && isEmpty(values[k]));

  const coerce = (key: string, raw: string): unknown => {
    const type = wfField(key)?.type;
    if (type === "number") return raw === "" ? null : Number(raw);
    if (type === "checkbox") return raw === "true";
    return raw;
  };

  async function run(t: AvailableTransition) {
    if (!detail) return;
    setBusy(t.key);
    setError(null);
    try {
      const fields: Record<string, unknown> = {};
      for (const k of t.requireFields) {
        if (!isEmpty(values[k])) fields[k] = coerce(k, values[k]);
      }
      await client.transition(submissionId, {
        transition: t.key,
        fields,
        expectedState: detail.submission.state,
      });
      setValues({});
      await load();
      onTransitioned();
    } catch (e) {
      setError(messageOf(e));
      await load();
    } finally {
      setBusy(null);
    }
  }

  return (
    <aside className="panel" aria-labelledby="reviewer-title">
      <h3 id="reviewer-title">Play the reviewer</h3>
      <div className="segmented" role="group" aria-label="Act as">
        {PERSONAS.map((p) => (
          <button
            key={p.email}
            type="button"
            aria-pressed={p.email === persona.email}
            onClick={() => setPersona(p)}
          >
            {p.label}
          </button>
        ))}
      </div>
      <p className="muted small">
        Signed in as <code>{persona.email}</code>. Guards in the workflow decide what this role may do.
      </p>

      {error && (
        <p className="error" role="alert">
          {error}
        </p>
      )}
      {!detail && !error && <p className="muted">Loading submission…</p>}

      {detail && detail.submission.terminal && (
        <p className="final">This submission reached a final state: {detail.submission.stateLabel}.</p>
      )}

      {detail && !detail.submission.terminal && detail.transitions.length === 0 && (
        <p className="muted">No transitions are available from {detail.submission.stateLabel}.</p>
      )}

      {detail && !detail.submission.terminal && (
        <ul className="transitions">
          {detail.transitions.map((t) => {
            const need = missing(t);
            return (
              <li key={t.key} className="transition">
                {t.allowed &&
                  t.requireFields
                    .filter((k) => isEmpty(detail.submission.fields[k]))
                    .map((k) => {
                      const f = wfField(k);
                      const id = `field-${t.key}-${k}`;
                      return (
                        <label key={k} htmlFor={id} className="field">
                          <span>{f?.label ?? k}</span>
                          {f?.type === "textarea" ? (
                            <textarea
                              id={id}
                              rows={2}
                              value={values[k] ?? ""}
                              onChange={(e) => setValues((v) => ({ ...v, [k]: e.target.value }))}
                            />
                          ) : (
                            <input
                              id={id}
                              type={f?.type === "number" ? "number" : "text"}
                              value={values[k] ?? ""}
                              onChange={(e) => setValues((v) => ({ ...v, [k]: e.target.value }))}
                            />
                          )}
                        </label>
                      );
                    })}
                <button
                  type="button"
                  className="button primary"
                  disabled={!t.allowed || need.length > 0 || busy !== null}
                  onClick={() => void run(t)}
                >
                  {t.label}
                </button>
                <span className="muted small">
                  {t.allowed ? `→ ${t.toLabel}` : `Not allowed for ${persona.label}`}
                </span>
              </li>
            );
          })}
        </ul>
      )}
    </aside>
  );
}
