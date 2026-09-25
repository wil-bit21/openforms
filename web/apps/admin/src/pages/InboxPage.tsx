import { useMemo } from "react";
import { useInfiniteQuery } from "@tanstack/react-query";
import { Link, useSearchParams } from "react-router-dom";
import { client } from "../api";
import { qk, type SubmissionFilter } from "../queryKeys";
import { useCatalog } from "../lib/catalog";
import { submissionSummary } from "../lib/format";
import { ErrorMessage } from "../components/ErrorMessage";
import { Loading } from "../components/Loading";
import { RelativeTime } from "../components/RelativeTime";
import { StateBadge } from "../components/StateBadge";

export const INBOX_REFRESH_MS = 30_000;
const PAGE_SIZE = 50;

const ASSIGNEE_OPTIONS = [
  { value: "", label: "Anyone" },
  { value: "me", label: "Assigned to me" },
  { value: "none", label: "Unassigned" },
];

export function InboxPage() {
  const catalog = useCatalog();
  const [params, setParams] = useSearchParams();
  const form = params.get("form") ?? "";
  const state = params.get("state") ?? "";
  const assignee = params.get("assignee") ?? "";

  const filter = useMemo(() => {
    const f: SubmissionFilter = {};
    if (form) f.form = form;
    if (state) f.state = state;
    if (assignee) f.assignee = assignee;
    return f;
  }, [form, state, assignee]);

  const query = useInfiniteQuery({
    queryKey: qk.submissions(filter),
    queryFn: ({ pageParam }) => client.listSubmissions({ ...filter, limit: PAGE_SIZE, ...(pageParam ? { cursor: pageParam } : {}) }),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.nextCursor ?? undefined,
    refetchInterval: INBOX_REFRESH_MS,
  });

  function update(key: "form" | "state" | "assignee", value: string) {
    const next = new URLSearchParams(params);
    if (value) next.set(key, value);
    else next.delete(key);
    if (key === "form") next.delete("state");
    setParams(next, { replace: true });
  }

  const states = form ? catalog.workflowDef(form)?.states ?? [] : [];
  const rows = query.data?.pages.flatMap((p) => p.items) ?? [];

  return (
    <div className="page">
      <header className="page-header">
        <h1>Inbox</h1>
      </header>

      <div className="toolbar" role="group" aria-label="Filters">
        <div className="field">
          <label htmlFor="filter-form">Form</label>
          <select id="filter-form" value={form} onChange={(e) => update("form", e.target.value)}>
            <option value="">All forms</option>
            {catalog.forms.map((f) => (
              <option key={f.slug} value={f.slug}>{f.title}</option>
            ))}
          </select>
        </div>
        <div className="field">
          <label htmlFor="filter-state">State</label>
          <select id="filter-state" value={state} disabled={states.length === 0} onChange={(e) => update("state", e.target.value)}>
            <option value="">Any state</option>
            {states.map((s) => (
              <option key={s.key} value={s.key}>{s.label}</option>
            ))}
          </select>
        </div>
        <div className="field">
          <label htmlFor="filter-assignee">Assignee</label>
          <select id="filter-assignee" value={assignee} onChange={(e) => update("assignee", e.target.value)}>
            {ASSIGNEE_OPTIONS.map((o) => (
              <option key={o.value} value={o.value}>{o.label}</option>
            ))}
          </select>
        </div>
      </div>

      {query.isPending ? (
        <Loading />
      ) : query.isError ? (
        <ErrorMessage error={query.error} />
      ) : rows.length === 0 ? (
        <p className="empty">No submissions match these filters.</p>
      ) : (
        <table className="table">
          <thead>
            <tr>
              <th>Received</th>
              <th>Form</th>
              <th>Summary</th>
              <th>State</th>
              <th>Assignee</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((s) => (
              <tr key={s.id}>
                <td><RelativeTime iso={s.createdAt} /></td>
                <td>{catalog.formTitle(s.form)}</td>
                <td>
                  <Link to={`/submissions/${s.id}`}>{submissionSummary(s.data, catalog.formDef(s.form))}</Link>
                </td>
                <td><StateBadge label={s.stateLabel} color={catalog.stateColor(s.form, s.state)} /></td>
                <td>{s.assignee?.name ?? <span className="muted">—</span>}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      {query.hasNextPage && (
        <div>
          <button type="button" className="btn" onClick={() => query.fetchNextPage()} disabled={query.isFetchingNextPage}>
            {query.isFetchingNextPage ? "Loading…" : "Load more"}
          </button>
        </div>
      )}
    </div>
  );
}
