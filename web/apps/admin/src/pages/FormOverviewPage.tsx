import { useQuery } from "@tanstack/react-query";
import { Link, useParams } from "react-router-dom";
import { client } from "../api";
import { qk } from "../queryKeys";
import { errorCode } from "../lib/errors";
import { isAdmin, useSession } from "../lib/session";
import { ErrorMessage } from "../components/ErrorMessage";
import { Loading } from "../components/Loading";
import { RelativeTime } from "../components/RelativeTime";
import { SourceTag, VersionTable } from "../components/VersionTable";
import { OverviewExtras } from "../extensions/OverviewExtras";

type ConditionLike = { field: string; equals?: unknown; notEquals?: unknown; in?: unknown[] };

export function conditionText(c?: ConditionLike | null): string {
  if (!c) return "Always";
  if (c.in !== undefined) return `${c.field} in [${c.in.map(String).join(", ")}]`;
  if (c.notEquals !== undefined) return `${c.field} ≠ ${String(c.notEquals)}`;
  return `${c.field} = ${String(c.equals)}`;
}

export function FormOverviewPage() {
  const { slug = "" } = useParams();
  const { data: me } = useSession();
  const form = useQuery({ queryKey: qk.form(slug), queryFn: () => client.getForm(slug) });
  const versions = useQuery({ queryKey: qk.formVersions(slug), queryFn: () => client.formVersions(slug) });

  if (form.isPending) return <div className="page"><Loading /></div>;
  if (form.isError) {
    return (
      <div className="page">
        {errorCode(form.error) === "not_found" ? (
          <>
            <h1>Form not found</h1>
            <p><Link to="/forms">Back to forms</Link></p>
          </>
        ) : (
          <ErrorMessage error={form.error} />
        )}
      </div>
    );
  }

  const record = form.data;
  const def = record.definition;
  const isPublic = !!def.settings?.public;

  return (
    <div className="page">
      <nav className="breadcrumb" aria-label="Breadcrumb">
        <Link to="/forms">Forms</Link> / <span>{def.title}</span>
      </nav>
      <header className="page-header">
        <div>
          <h1>{def.title}</h1>
          {def.description && <p className="muted">{def.description}</p>}
        </div>
        {isAdmin(me) && <Link className="btn btn-primary" to={`/forms/${slug}/edit`}>Edit</Link>}
      </header>

      <dl className="meta">
        <div><dt>Slug</dt><dd className="mono">{def.slug}</dd></div>
        <div><dt>Version</dt><dd>v{record.version} <SourceTag source={record.source} /></dd></div>
        <div>
          <dt>Workflow</dt>
          <dd>
            {def.workflow ? (
              <>
                <Link to={`/workflows/${def.workflow}`}>{def.workflow}</Link>
                {record.workflowVersion != null && <span className="muted"> (v{record.workflowVersion})</span>}
              </>
            ) : (
              "None"
            )}
          </dd>
        </div>
        <div><dt>Public</dt><dd>{isPublic ? "Yes" : "No"}</dd></div>
        <div><dt>Updated</dt><dd><RelativeTime iso={record.updatedAt} /></dd></div>
      </dl>

      <div className="button-row">
        <Link className="btn" to={`/submissions?form=${encodeURIComponent(slug)}`}>View submissions</Link>
        {isPublic && <a className="btn" href={`/f/${slug}`} target="_blank" rel="noreferrer">Open hosted form</a>}
        <a className="btn" href={client.csvUrl(slug)} download>Download CSV</a>
      </div>

      <section className="card" aria-label="Fields">
        <h2>Fields</h2>
        <table className="table">
          <thead>
            <tr>
              <th>Key</th>
              <th>Label</th>
              <th>Type</th>
              <th>Required</th>
              <th>Shown when</th>
            </tr>
          </thead>
          <tbody>
            {def.fields.map((f) => (
              <tr key={f.key}>
                <td className="mono">{f.key}</td>
                <td>{f.label}</td>
                <td>{f.type}</td>
                <td>{f.required ? "Yes" : "No"}</td>
                <td>{conditionText(f.showIf as ConditionLike | undefined)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </section>

      <section className="card" aria-label="Versions">
        <h2>Versions</h2>
        {versions.isPending ? <Loading /> : versions.isError ? <ErrorMessage error={versions.error} /> : <VersionTable versions={versions.data} />}
      </section>

      <OverviewExtras kind="form" slug={slug} />
    </div>
  );
}
