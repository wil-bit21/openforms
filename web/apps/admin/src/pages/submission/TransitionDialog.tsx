import { useState, type FormEvent } from "react";
import type { AvailableTransition, Submission, WorkflowDefinition } from "@openforms/sdk";
import { Dialog } from "../../components/Dialog";
import { ErrorMessage } from "../../components/ErrorMessage";
import { problemsByPath } from "../../lib/errors";
import { isEmpty, WorkflowFieldInput, type WorkflowFieldLike } from "./WorkflowFieldInput";

export function TransitionDialog({ transition, workflow, submission, pending, serverError, onCancel, onConfirm }: {
  transition: AvailableTransition;
  workflow: WorkflowDefinition;
  submission: Submission;
  pending: boolean;
  serverError: unknown;
  onCancel: () => void;
  onConfirm: (input: { fields: Record<string, unknown>; comment: string }) => void;
}) {
  const required: WorkflowFieldLike[] = transition.requireFields.map(
    (key) => workflow.fields?.find((f) => f.key === key) ?? { key, type: "text", label: key },
  );
  const [values, setValues] = useState<Record<string, unknown>>(() =>
    Object.fromEntries(required.map((f) => [f.key, submission.fields[f.key] ?? null])),
  );
  const [comment, setComment] = useState("");
  const [missing, setMissing] = useState<string[]>([]);
  const problems = problemsByPath(serverError);

  function submit(e: FormEvent) {
    e.preventDefault();
    const miss = required.filter((f) => isEmpty(values[f.key])).map((f) => f.key);
    setMissing(miss);
    if (miss.length) return;
    const fields = Object.fromEntries(Object.entries(values).map(([k, v]) => [k, v === "" ? null : v]));
    onConfirm({ fields, comment: comment.trim() });
  }

  return (
    <Dialog title={transition.label} onClose={onCancel}>
      <form onSubmit={submit} noValidate>
        <p className="muted">Moves this submission to <strong>{transition.toLabel}</strong>.</p>
        {required.map((f) => (
          <WorkflowFieldInput
            key={f.key}
            idPrefix="tr"
            field={f}
            value={values[f.key]}
            onChange={(v) => setValues((s) => ({ ...s, [f.key]: v }))}
            error={missing.includes(f.key) ? "Required" : problems[`fields.${f.key}`]}
          />
        ))}
        <div className="field">
          <label htmlFor="tr-comment">Comment (optional)</label>
          <textarea id="tr-comment" rows={3} value={comment} onChange={(e) => setComment(e.target.value)} />
        </div>
        {serverError && Object.keys(problems).length === 0 ? <ErrorMessage error={serverError} /> : null}
        <div className="dialog-footer">
          <button type="button" className="btn" onClick={onCancel}>Cancel</button>
          <button type="submit" className="btn btn-primary" disabled={pending}>{transition.label}</button>
        </div>
      </form>
    </Dialog>
  );
}
