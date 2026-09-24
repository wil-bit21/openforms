import type { SubmissionEvent, WorkflowDefinition } from "@openforms/sdk";
import { describeEvent } from "../../lib/events";
import { RelativeTime } from "../../components/RelativeTime";

export function Timeline({ events, workflow }: { events: SubmissionEvent[]; workflow?: WorkflowDefinition | null }) {
  const ordered = [...events].sort((a, b) => b.id - a.id);
  if (ordered.length === 0) return <p className="muted">No activity yet.</p>;
  return (
    <ol className="timeline" aria-label="Activity">
      {ordered.map((e) => {
        const view = describeEvent(e, workflow);
        return (
          <li key={e.id} className={`timeline-item tone-${view.tone}`} data-type={e.type}>
            <div className="timeline-title">{view.title}</div>
            {view.body && <p className="timeline-body">{view.body}</p>}
            <RelativeTime iso={e.createdAt} />
          </li>
        );
      })}
    </ol>
  );
}
