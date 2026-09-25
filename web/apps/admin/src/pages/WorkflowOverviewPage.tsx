import { useQuery } from "@tanstack/react-query";
import { Link, useParams } from "react-router-dom";
import { client } from "../api";
import { qk } from "../queryKeys";
import { errorCode } from "../lib/errors";
import { isAdmin, useSession } from "../lib/session";
import { ErrorMessage } from "../components/ErrorMessage";
import { Loading } from "../components/Loading";
import { RelativeTime } from "../components/RelativeTime";
import { StateBadge } from "../components/StateBadge";
import { SourceTag, VersionTable } from "../components/VersionTable";
import { OverviewExtras } from "../extensions/OverviewExtras";

type ActionLike = { type: string; url?: string; to?: string; user?: string; role?: string };

export function actionText(a: ActionLike): string {
  switch (a.type) {
    case "webhook": return `Webhook to ${a.url ?? ""}`;
    case "email": return `Email to ${a.to ?? ""}`;
    case "assign": return a.user ? `Assign to ${a.user}` : `Assign to role ${a.role ?? ""}`;
    default: return a.type;
  }
}

export function WorkflowOverviewPage() {
  const { slug = "" } = useParams();
  const { data: me } = useSession();
  const workflow = useQuery({ queryKey: qk.workflow(slug), queryFn: () => client.getWorkflow(slug) });
  const versions = useQuery({ queryKey: qk.workflowVersions(slug), queryFn: () => client.workflowVersions(slug) });
  const forms = useQuery({ queryKey: qk.forms, queryFn: () => client.listForms() });

  if (workflow.isPending) return <div className="page"><Loading /></div>;
  if (workflow.isError) {
    return (
      <div className="page">
        {errorCode(workflow.error) === "not_found" ? (
          <>
            <h1>Workflow not found</h1>
            <p><Link to="/workflows">Back to workflows</Link></p>
          </>
        ) : (
          <ErrorMessage error={workflow.error} />
        )}
      </div>
    );
  }

  const record = workflow.data;
  const def = record.definition;
  const stateLabel = (key: string) => def.states.find((s) => s.key === key)?.label ?? key;
  const fieldLabel = (key: string) => def.fields?.find((f) => f.key === key)?.label ?? key;
  const usedBy = (forms.data ?? []).filter((f) => f.workflow === slug);

  return (
    <div className="page">
      <nav className="breadcrumb" aria-label="Breadcrumb">
        <Link to="/workflows">Workflows</Link> / <span>{def.title}</span>
      </nav>
      <header className="page-header">
        <h1>{def.title}</h1>
        {isAdmin(me) && <Link className="btn btn-primary" to={`/workflows/${slug}/edit`}>Edit</Link>}
      </header>

      <dl className="meta">
        <div><dt>Slug</dt><dd className="mono">{def.slug}</dd></div>
        <div><dt>Version</dt><dd>v{record.version} <SourceTag source={record.source} /></dd></div>
        <div><dt>Updated</dt><dd><RelativeTime iso={record.updatedAt} /></dd></div>
      </dl>

      <section className="card" aria-label="States">
        <h2>States</h2>
        <ul className="button-row" style={{ listStyle: "none", margin: 0, padding: 0 }}>
          {def.states.map((s) => (
            <li key={s.key}>
              <StateBadge label={s.label} color={s.color} />{" "}
              {s.key === def.initial && <span className="muted">initial</span>}
              {s.terminal && <span className="muted">terminal</span>}
            </li>
          ))}
        </ul>
      </section>

      <section className="card" aria-label="Transitions">
        <h2>Transitions</h2>
        <table className="table">
          <thead>
            <tr>
              <th>Transition</th>
              <th>From</th>
              <th>To</th>
              <th>Roles</th>
              <th>Required fields</th>
              <th>Actions</th>
            </tr>
          </thead>
          <tbody>
            {(def.transitions ?? []).map((t) => (
              <tr key={t.key}>
                <td>{t.label}</td>
                <td>{t.from.map(stateLabel).join(", ")}</td>
                <td>{stateLabel(t.to)}</td>
                <td>{t.guard?.roles?.length ? t.guard.roles.join(", ") : <span className="muted">Anyone</span>}</td>
                <td>{t.guard?.requireFields?.length ? t.guard.requireFields.map(fieldLabel).join(", ") : <span className="muted">—</span>}</td>
                <td>{t.actions?.length ? t.actions.map((a) => actionText(a as ActionLike)).join("; ") : <span className="muted">—</span>}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </section>

      <section className="card" aria-label="Workflow fields">
        <h2>Workflow fields</h2>
        {def.fields?.length ? (
          <ul>
            {def.fields.map((f) => (
              <li key={f.key}><span>{f.label}</span> <span className="muted mono">{f.key} · {f.type}</span></li>
            ))}
          </ul>
        ) : (
          <p className="muted">None.</p>
        )}
      </section>

      <section className="card" aria-label="On submit">
        <h2>On submit</h2>
        {def.onSubmit?.length ? (
          <ul>
            {def.onSubmit.map((a, i) => <li key={i}>{actionText(a as ActionLike)}</li>)}
          </ul>
        ) : (
          <p className="muted">No actions.</p>
        )}
      </section>

      <section className="card" aria-label="Used by forms">
        <h2>Used by forms</h2>
        {forms.isPending ? (
          <Loading />
        ) : usedBy.length ? (
          <ul>
            {usedBy.map((f) => <li key={f.slug}><Link to={`/forms/${f.slug}`}>{f.title}</Link></li>)}
          </ul>
        ) : (
          <p className="muted">No forms use this workflow yet.</p>
        )}
      </section>

      <section className="card" aria-label="Versions">
        <h2>Versions</h2>
        {versions.isPending ? <Loading /> : versions.isError ? <ErrorMessage error={versions.error} /> : <VersionTable versions={versions.data} />}
      </section>

      <OverviewExtras kind="workflow" slug={slug} />
    </div>
  );
}
