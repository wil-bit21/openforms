import { useState, type FormEvent } from "react";
import { useMutation } from "@tanstack/react-query";
import type { SubmissionDetail } from "@openforms/sdk";
import { client } from "../../api";
import { ErrorMessage } from "../../components/ErrorMessage";
import { problemsByPath } from "../../lib/errors";
import { isEmpty, WorkflowFieldInput } from "./WorkflowFieldInput";
import { useRefreshSubmission } from "./refresh";

const same = (a: unknown, b: unknown) => (isEmpty(a) && isEmpty(b)) || a === b;

/** Re-mount with `key={submission.updatedAt}` so a refetch resets local edits. */
export function WorkflowFieldsPanel({ detail }: { detail: SubmissionDetail }) {
  const { submission, workflow } = detail;
  const defs = workflow?.fields ?? [];
  const refresh = useRefreshSubmission(submission.id);
  const [values, setValues] = useState<Record<string, unknown>>(() => ({ ...submission.fields }));

  const changed: Record<string, unknown> = {};
  for (const f of defs) {
    if (!same(values[f.key], submission.fields[f.key])) changed[f.key] = isEmpty(values[f.key]) ? null : values[f.key];
  }
  const dirty = Object.keys(changed).length > 0;

  const mutation = useMutation({
    mutationFn: () => client.updateFields(submission.id, changed),
    onSuccess: () => refresh(),
  });
  const problems = problemsByPath(mutation.error);

  if (defs.length === 0) return null;

  function submit(e: FormEvent) {
    e.preventDefault();
    if (dirty) mutation.mutate();
  }

  return (
    <section className="card" aria-label="Workflow fields">
      <h2>Workflow fields</h2>
      <form onSubmit={submit}>
        {defs.map((f) => (
          <WorkflowFieldInput
            key={f.key}
            idPrefix="wf"
            field={f}
            value={values[f.key]}
            onChange={(v) => setValues((s) => ({ ...s, [f.key]: v }))}
            error={problems[`fields.${f.key}`]}
          />
        ))}
        {mutation.isError && Object.keys(problems).length === 0 && <ErrorMessage error={mutation.error} />}
        <button type="submit" className="btn btn-primary" disabled={!dirty || mutation.isPending}>
          {mutation.isPending ? "Saving…" : "Save fields"}
        </button>
      </form>
    </section>
  );
}
