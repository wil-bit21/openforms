import { useQuery } from "@tanstack/react-query";
import { Link, useParams } from "react-router-dom";
import { client } from "../api";
import { qk } from "../queryKeys";
import { errorCode } from "../lib/errors";
import { shortId } from "../lib/format";
import { ErrorMessage } from "../components/ErrorMessage";
import { Loading } from "../components/Loading";
import { RelativeTime } from "../components/RelativeTime";
import { StateBadge } from "../components/StateBadge";
import { AnswersPanel } from "./submission/AnswersPanel";
import { AssigneePicker } from "./submission/AssigneePicker";
import { CommentBox } from "./submission/CommentBox";
import { Timeline } from "./submission/Timeline";
import { TransitionBar } from "./submission/TransitionBar";
import { WorkflowFieldsPanel } from "./submission/WorkflowFieldsPanel";

export function SubmissionPage() {
  const { id = "" } = useParams();
  const query = useQuery({ queryKey: qk.submission(id), queryFn: () => client.getSubmission(id) });

  if (query.isPending) return <div className="page"><Loading /></div>;
  if (query.isError) {
    return (
      <div className="page">
        {errorCode(query.error) === "not_found" ? (
          <>
            <h1>Submission not found</h1>
            <p><Link to="/submissions">Back to the inbox</Link></p>
          </>
        ) : (
          <ErrorMessage error={query.error} />
        )}
      </div>
    );
  }

  const detail = query.data;
  const { submission, form, workflow, events } = detail;
  const color = workflow?.states.find((s) => s.key === submission.state)?.color;

  return (
    <div className="page">
      <nav className="breadcrumb" aria-label="Breadcrumb">
        <Link to="/submissions">Inbox</Link> / <span>{form.title}</span>
      </nav>
      <header className="page-header">
        <div>
          <h1>
            {form.title} <span className="muted mono">#{shortId(submission.id)}</span>
          </h1>
          <p className="muted">
            Received <RelativeTime iso={submission.createdAt} /> · form v{submission.formVersion}
          </p>
        </div>
        <StateBadge label={submission.stateLabel} color={color} />
      </header>
      <div className="detail">
        <div className="detail-main">
          <section className="card" aria-label="Answers">
            <h2>Answers</h2>
            <AnswersPanel form={form} data={submission.data} />
          </section>
          <section className="card" aria-label="Activity log">
            <h2>Activity</h2>
            <CommentBox submissionId={submission.id} />
            <Timeline events={events} workflow={workflow} />
          </section>
        </div>
        <aside className="detail-side">
          <section className="card" aria-label="Actions">
            <h2>Actions</h2>
            <TransitionBar detail={detail} />
          </section>
          <AssigneePicker submission={submission} />
          <WorkflowFieldsPanel key={submission.updatedAt} detail={detail} />
        </aside>
      </div>
    </div>
  );
}
