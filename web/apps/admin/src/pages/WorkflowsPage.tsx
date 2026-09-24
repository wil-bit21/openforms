import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { client } from "../api";
import { qk } from "../queryKeys";
import { isAdmin, useSession } from "../lib/session";
import { ErrorMessage } from "../components/ErrorMessage";
import { Loading } from "../components/Loading";
import { RelativeTime } from "../components/RelativeTime";
import { SourceTag } from "../components/VersionTable";

export function WorkflowsPage() {
  const { data: me } = useSession();
  const query = useQuery({ queryKey: qk.workflows, queryFn: () => client.listWorkflows() });

  return (
    <div className="page">
      <header className="page-header">
        <h1>Workflows</h1>
        {isAdmin(me) && <Link className="btn btn-primary" to="/workflows/new">New workflow</Link>}
      </header>
      {query.isPending ? (
        <Loading />
      ) : query.isError ? (
        <ErrorMessage error={query.error} />
      ) : query.data.length === 0 ? (
        <p className="empty">No workflows yet. Forms without a workflow keep every submission in the “Submitted” state.</p>
      ) : (
        <table className="table">
          <thead>
            <tr>
              <th>Title</th>
              <th>Slug</th>
              <th>States</th>
              <th>Version</th>
              <th>Updated</th>
            </tr>
          </thead>
          <tbody>
            {query.data.map((w) => (
              <tr key={w.slug}>
                <td><Link to={`/workflows/${w.slug}`}>{w.title}</Link></td>
                <td className="mono">{w.slug}</td>
                <td>{w.stateCount}</td>
                <td><span>v{w.version}</span> <SourceTag source={w.source} /></td>
                <td><RelativeTime iso={w.updatedAt} /></td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
