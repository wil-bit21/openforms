import { describe, expect, it } from "vitest";
import { makeEvent, makeWorkflowDefinition } from "../test/fixtures";
import { actorName, describeEvent } from "./events";

const wf = makeWorkflowDefinition();
const rita = { type: "user", id: "u1", name: "Rita Reviewer" } as const;

describe("describeEvent", () => {
  it("describes creation by a respondent or a user", () => {
    expect(describeEvent(makeEvent(), wf)).toEqual({ title: "Submitted", tone: "default" });
    expect(describeEvent(makeEvent({ actor: rita }), wf).title).toBe("Created by Rita Reviewer");
  });

  it("describes transitions with state labels and comment", () => {
    const e = makeEvent({ id: 2, type: "transition", fromState: "new", toState: "screening", transition: "screen", actor: rita, payload: { comment: "Looks promising" } });
    expect(describeEvent(e, wf)).toEqual({ title: "Rita Reviewer: Start screening (New → Screening)", body: "Looks promising", tone: "default" });
  });

  it("falls back to raw keys without a workflow", () => {
    const e = makeEvent({ type: "transition", fromState: "a", toState: "b", transition: "go", actor: rita });
    expect(describeEvent(e, null).title).toBe("Rita Reviewer: go (a → b)");
  });

  it("describes field updates, assignment, comments and actions", () => {
    expect(describeEvent(makeEvent({ type: "fields_updated", actor: rita, payload: { fields: { score: 4 } } }), wf).title).toBe("Rita Reviewer updated Score");
    expect(describeEvent(makeEvent({ type: "assigned", actor: { type: "system", id: null, name: "" }, payload: { assigneeId: "u1", assigneeName: "Rita Reviewer" } }), wf).title).toBe("openforms assigned this to Rita Reviewer");
    expect(describeEvent(makeEvent({ type: "assigned", actor: rita, payload: { assigneeId: null } }), wf).title).toBe("Rita Reviewer removed the assignee");
    expect(describeEvent(makeEvent({ type: "comment", actor: rita, payload: { body: "Call on Monday" } }), wf)).toEqual({ title: "Rita Reviewer commented", body: "Call on Monday", tone: "default" });
    expect(describeEvent(makeEvent({ type: "action_succeeded", actor: { type: "system", id: null, name: "" }, payload: { action: { type: "webhook" } } }), wf).title).toBe("Webhook action succeeded");
    expect(describeEvent(makeEvent({ type: "action_failed", actor: { type: "system", id: null, name: "" }, payload: { action: { type: "email" }, error: "SMTP timeout" } }), wf))
      .toEqual({ title: "Email action failed", body: "SMTP timeout", tone: "danger" });
  });

  it("names actors when the event has no name", () => {
    expect(actorName(makeEvent({ actor: { type: "respondent", id: null, name: "" } }))).toBe("Respondent");
    expect(actorName(makeEvent({ actor: { type: "system", id: null, name: "" } }))).toBe("openforms");
    expect(actorName(makeEvent({ actor: { type: "api_key", id: "k", name: "" } }))).toBe("API key");
  });
});
