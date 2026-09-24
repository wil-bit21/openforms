import { useMutation, useQuery } from "@tanstack/react-query";
import type { Submission } from "@openforms/sdk";
import { client } from "../../api";
import { qk } from "../../queryKeys";
import { ErrorMessage } from "../../components/ErrorMessage";
import { isAdmin, useSession } from "../../lib/session";
import { useRefreshSubmission } from "./refresh";

export function AssigneePicker({ submission }: { submission: Submission }) {
  const { data: me } = useSession();
  const admin = isAdmin(me);
  const refresh = useRefreshSubmission(submission.id);
  // GET /users is admin-only; reviewers get self-assignment buttons instead.
  const users = useQuery({ queryKey: qk.users, queryFn: () => client.listUsers(), enabled: admin });
  const mutation = useMutation({
    mutationFn: (userId: string | null) => client.assign(submission.id, userId),
    onSuccess: () => refresh(),
  });
  const current = submission.assignee;

  return (
    <section className="card" aria-label="Assignee">
      <h2>Assignee</h2>
      <p>
        {current ? (
          <>
            <strong>{current.name}</strong> <span className="muted">{current.email}</span>
          </>
        ) : (
          <span className="muted">Unassigned</span>
        )}
      </p>
      {admin && users.data ? (
        <div className="field">
          <label htmlFor="assignee-select">Assign to</label>
          <select id="assignee-select" value={current?.id ?? ""} disabled={mutation.isPending}
            onChange={(e) => mutation.mutate(e.target.value || null)}>
            <option value="">Unassigned</option>
            {users.data.map((u) => (
              <option key={u.id} value={u.id}>{u.name} ({u.email})</option>
            ))}
          </select>
        </div>
      ) : (
        <div className="button-row">
          {me?.kind === "user" && current?.id !== me.id && (
            <button type="button" className="btn" disabled={mutation.isPending} onClick={() => mutation.mutate(me.id)}>
              Assign to me
            </button>
          )}
          {current && (
            <button type="button" className="btn btn-ghost" disabled={mutation.isPending} onClick={() => mutation.mutate(null)}>
              Unassign
            </button>
          )}
        </div>
      )}
      {mutation.isError && <ErrorMessage error={mutation.error} />}
    </section>
  );
}
