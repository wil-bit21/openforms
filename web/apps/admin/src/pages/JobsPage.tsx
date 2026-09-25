import type { JobStatus } from "@openforms/sdk";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useSearchParams } from "react-router-dom";
import { client } from "../api";
import { qk } from "../queryKeys";
import { shortId } from "../lib/format";
import { ErrorMessage } from "../components/ErrorMessage";
import { Loading } from "../components/Loading";
import { RelativeTime } from "../components/RelativeTime";

const STATUSES = [
  { value: "failed", label: "Failed" },
  { value: "pending", label: "Pending" },
  { value: "running", label: "Running" },
  { value: "done", label: "Done" },
];

export function JobsPage() {
  const [params, setParams] = useSearchParams();
  const status = STATUSES.some((s) => s.value === params.get("status")) ? (params.get("status") as JobStatus) : "failed";
  const queryClient = useQueryClient();
  const query = useQuery({ queryKey: qk.jobs(status), queryFn: () => client.listJobs(status) });
  const retry = useMutation({
    mutationFn: (id: number) => client.retryJob(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["jobs"] }),
  });

  return (
    <div className="page">
      <header className="page-header">
        <div>
          <h1>Jobs</h1>
          <p className="muted">Webhook, email and assignment actions run in the background with retries.</p>
        </div>
        <button type="button" className="btn" onClick={() => query.refetch()}>Refresh</button>
      </header>

      <div className="tabs" role="group" aria-label="Job status">
        {STATUSES.map((s) => (
          <button key={s.value} type="button" className="btn" aria-pressed={s.value === status}
            onClick={() => setParams(s.value === "failed" ? {} : { status: s.value }, { replace: true })}>
            {s.label}
          </button>
        ))}
      </div>

      {retry.isError && <ErrorMessage error={retry.error} />}
      {query.isPending ? (
        <Loading />
      ) : query.isError ? (
        <ErrorMessage error={query.error} />
      ) : query.data.length === 0 ? (
        <p className="empty">No {status} jobs.</p>
      ) : (
        <table className="table">
          <thead>
            <tr>
              <th>ID</th>
              <th>Kind</th>
              <th>Submission</th>
              <th>Attempts</th>
              <th>Next run</th>
              <th>Last error</th>
              <th><span className="sr-only">Actions</span></th>
            </tr>
          </thead>
          <tbody>
            {query.data.map((job) => {
              const payload = (job.payload ?? {}) as Record<string, unknown>;
              const submissionId = typeof payload.submissionId === "string" ? payload.submissionId : null;
              return (
                <tr key={job.id}>
                  <td className="mono">{job.id}</td>
                  <td className="mono">{job.kind}</td>
                  <td>{submissionId ? <Link className="mono" to={`/submissions/${submissionId}`}>{shortId(submissionId)}</Link> : <span className="muted">—</span>}</td>
                  <td>{job.attempts}/{job.maxAttempts}</td>
                  <td><RelativeTime iso={job.runAt} /></td>
                  <td title={job.lastError}>{job.lastError ? job.lastError.slice(0, 140) : <span className="muted">—</span>}</td>
                  <td>
                    {job.status === "failed" && (
                      <button type="button" className="btn" disabled={retry.isPending} onClick={() => retry.mutate(job.id)}>Retry</button>
                    )}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      )}
    </div>
  );
}
