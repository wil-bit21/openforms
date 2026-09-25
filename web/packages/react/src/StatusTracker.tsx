import { OpenFormsError, type OpenFormsClient, type PublicStatus, type State } from "@openforms/sdk";
import { useEffect, useState } from "react";

export interface StatusTrackerProps {
  client: OpenFormsClient;
  submissionId: string;
  token: string;
  /** Poll interval in ms while the submission is not terminal. Default 5000. */
  pollMs?: number;
  className?: string;
}

const cx = (...names: Array<string | false | null | undefined>) => names.filter(Boolean).join(" ");

function isNotFound(err: unknown) {
  return err instanceof OpenFormsError && err.status === 404;
}

/** Non-terminal states in order, plus the current state when it is terminal. */
function visibleSteps(status: PublicStatus): State[] {
  const steps = status.states.filter((s) => !s.terminal || s.key === status.state);
  if (!steps.some((s) => s.key === status.state)) {
    steps.push({ key: status.state, label: status.stateLabel, terminal: status.terminal });
  }
  return steps;
}

export function StatusTracker({ client, submissionId, token, pollMs = 5000, className }: StatusTrackerProps) {
  const [status, setStatus] = useState<PublicStatus | null>(null);
  const [error, setError] = useState<unknown>(null);

  useEffect(() => {
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout> | undefined;

    const tick = async () => {
      try {
        const next = await client.getPublicStatus(submissionId, token);
        if (cancelled) return;
        setStatus(next);
        setError(null);
        if (!next.terminal) timer = setTimeout(tick, pollMs);
      } catch (err) {
        if (cancelled) return;
        setError(err);
        if (!isNotFound(err)) timer = setTimeout(tick, pollMs);
      }
    };

    setStatus(null);
    setError(null);
    void tick();
    return () => {
      cancelled = true;
      if (timer !== undefined) clearTimeout(timer);
    };
  }, [client, submissionId, token, pollMs]);

  if (isNotFound(error)) {
    return (
      <div className={cx("of-status", className)} role="alert">
        <p>
          <strong>We couldn't find this submission.</strong> Check that you opened the full link from your
          confirmation.
        </p>
      </div>
    );
  }

  if (!status) {
    return (
      <div className={cx("of-status", className)} aria-busy="true">
        {error ? "We couldn't load the status yet. Retrying…" : "Loading status…"}
      </div>
    );
  }

  const visited = new Set(status.history.map((h) => h.state));

  return (
    <div className={cx("of-status", className)}>
      <h2 className="of-status-title">{status.formTitle}</h2>
      <p className="of-status-current" aria-live="polite">
        Current status: <strong>{status.stateLabel}</strong>
      </p>

      <ol className="of-status-chain" aria-label="Progress">
        {visibleSteps(status).map((step) => {
          const current = step.key === status.state;
          return (
            <li
              key={step.key}
              className={cx(
                "of-step",
                step.color && `of-color-${step.color}`,
                current && "of-step-current",
                !current && visited.has(step.key) && "of-step-done",
              )}
              aria-current={current ? "step" : undefined}
            >
              {step.label}
            </li>
          );
        })}
      </ol>

      <ol className="of-status-history" aria-label="History">
        {status.history.map((item, i) => (
          <li key={`${item.state}-${i}`}>
            <span>{item.label}</span>
            <time dateTime={item.at}>{new Date(item.at).toLocaleString()}</time>
          </li>
        ))}
      </ol>

      {error ? <p className="of-status-note">Couldn't refresh the status. Retrying…</p> : null}
    </div>
  );
}
