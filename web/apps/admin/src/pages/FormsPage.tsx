import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { client } from "../api";
import { qk } from "../queryKeys";
import { isAdmin, useSession } from "../lib/session";
import { ErrorMessage } from "../components/ErrorMessage";
import { Loading } from "../components/Loading";
import { RelativeTime } from "../components/RelativeTime";
import { SourceTag } from "../components/VersionTable";

export function FormsPage() {
  const { data: me } = useSession();
  const query = useQuery({ queryKey: qk.forms, queryFn: () => client.listForms() });

  return (
    <div className="page">
      <header className="page-header">
        <h1>Forms</h1>
        {isAdmin(me) && <Link className="btn btn-primary" to="/forms/new">New form</Link>}
      </header>
      {query.isPending ? (
        <Loading />
      ) : query.isError ? (
        <ErrorMessage error={query.error} />
      ) : query.data.length === 0 ? (
        <p className="empty">
          No forms yet. Define one in YAML and run <code>openforms push</code>, or create one here.
        </p>
      ) : (
        <table className="table">
          <thead>
            <tr>
              <th>Title</th>
              <th>Slug</th>
              <th>Workflow</th>
              <th>Public</th>
              <th>Version</th>
              <th>Submissions</th>
              <th>Updated</th>
            </tr>
          </thead>
          <tbody>
            {query.data.map((f) => (
              <tr key={f.slug}>
                <td><Link to={`/forms/${f.slug}`}>{f.title}</Link></td>
                <td className="mono">{f.slug}</td>
                <td>{f.workflow ? <Link to={`/workflows/${f.workflow}`}>{f.workflow}</Link> : <span className="muted">—</span>}</td>
                <td>{f.public ? "Yes" : "No"}</td>
                <td><span>v{f.version}</span> <SourceTag source={f.source} /></td>
                <td>{f.submissionCount}</td>
                <td><RelativeTime iso={f.updatedAt} /></td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
