import { useState } from "react";
import { useMutation } from "@tanstack/react-query";
import type { AvailableTransition, SubmissionDetail } from "@openforms/sdk";
import { client } from "../../api";
import { ErrorMessage } from "../../components/ErrorMessage";
import { errorCode } from "../../lib/errors";
import { useRefreshSubmission } from "./refresh";
import { TransitionDialog } from "./TransitionDialog";

export function reasonText(reason: string): string {
  return reason === "role" ? "You don't have a role that can perform this transition." : "This transition isn't available.";
}

type Vars = { t: AvailableTransition; fields?: Record<string, unknown>; comment?: string };

export function TransitionBar({ detail }: { detail: SubmissionDetail }) {
  const { submission, workflow, transitions } = detail;
  const refresh = useRefreshSubmission(submission.id);
  const [active, setActive] = useState<AvailableTransition | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const mutation = useMutation({
    mutationFn: ({ t, fields, comment }: Vars) =>
      client.transition(submission.id, {
        transition: t.key,
        expectedState: submission.state,
        ...(fields && Object.keys(fields).length ? { fields } : {}),
        ...(comment ? { comment } : {}),
      }),
    onSuccess: async () => {
      setActive(null);
      setNotice(null);
      await refresh();
    },
    onError: async (err) => {
      const code = errorCode(err);
      if (code === "state_conflict" || code === "invalid_state") {
        setActive(null);
        setNotice("This submission changed while you were viewing it.");
        await refresh();
      }
    },
  });

  if (!workflow) return <p className="muted">This form has no workflow.</p>;

  return (
    <div>
      {notice && <p role="alert" className="notice">{notice}</p>}
      {transitions.length === 0 ? (
        <p className="muted">No transitions from {submission.stateLabel}.</p>
      ) : (
        <div className="transition-buttons">
          {transitions.map((t) => (
            <button
              key={t.key}
              type="button"
              className="btn"
              disabled={!t.allowed || mutation.isPending}
              title={t.allowed ? `Move to ${t.toLabel}` : reasonText(t.reason)}
              onClick={() => {
                setNotice(null);
                mutation.reset();
                if (t.requireFields.length) setActive(t);
                else mutation.mutate({ t });
              }}
            >
              {t.label}
            </button>
          ))}
        </div>
      )}
      {mutation.isError && !active && !notice && <ErrorMessage error={mutation.error} />}
      {active && (
        <TransitionDialog
          transition={active}
          workflow={workflow}
          submission={submission}
          pending={mutation.isPending}
          serverError={mutation.error ?? undefined}
          onCancel={() => {
            setActive(null);
            mutation.reset();
          }}
          onConfirm={({ fields, comment }) => mutation.mutate({ t: active, fields, comment })}
        />
      )}
    </div>
  );
}
