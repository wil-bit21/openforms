import type { SubmissionEvent, WorkflowDefinition } from "@openforms/sdk";

export type EventView = { title: string; body?: string; tone: "default" | "danger" };

export function actorName(e: SubmissionEvent): string {
  if (e.actor.name) return e.actor.name;
  switch (e.actor.type) {
    case "respondent": return "Respondent";
    case "system": return "openforms";
    case "api_key": return "API key";
    default: return "Someone";
  }
}

function actionLabel(payload: Record<string, unknown>): string {
  const type = (payload.action as { type?: string } | undefined)?.type;
  return type ? `${type.charAt(0).toUpperCase()}${type.slice(1)} action` : "Action";
}

export function describeEvent(e: SubmissionEvent, wf?: WorkflowDefinition | null): EventView {
  const who = actorName(e);
  const p = (e.payload ?? {}) as Record<string, unknown>;
  const stateLabel = (key?: string | null) => (key ? wf?.states.find((s) => s.key === key)?.label ?? key : "—");

  switch (e.type) {
    case "created":
      return { title: e.actor.type === "respondent" ? "Submitted" : `Created by ${who}`, tone: "default" };
    case "transition": {
      const label = wf?.transitions?.find((t) => t.key === e.transition)?.label ?? e.transition ?? "Transition";
      const view: EventView = { title: `${who}: ${label} (${stateLabel(e.fromState)} → ${stateLabel(e.toState)})`, tone: "default" };
      if (typeof p.comment === "string" && p.comment) view.body = p.comment;
      return view;
    }
    case "fields_updated": {
      const keys = Object.keys((p.fields as Record<string, unknown> | undefined) ?? {});
      const labels = keys.map((k) => wf?.fields?.find((f) => f.key === k)?.label ?? k);
      return { title: `${who} updated ${labels.length ? labels.join(", ") : "fields"}`, tone: "default" };
    }
    case "assigned": {
      const name = typeof p.assigneeName === "string" && p.assigneeName ? p.assigneeName : null;
      return { title: name ? `${who} assigned this to ${name}` : `${who} removed the assignee`, tone: "default" };
    }
    case "comment":
      return { title: `${who} commented`, body: typeof p.body === "string" ? p.body : "", tone: "default" };
    case "action_succeeded":
      return { title: `${actionLabel(p)} succeeded`, tone: "default" };
    case "action_failed": {
      const view: EventView = { title: `${actionLabel(p)} failed`, tone: "danger" };
      if (typeof p.error === "string") view.body = p.error;
      return view;
    }
    default:
      return { title: e.type, tone: "default" };
  }
}
