import { describe, expect, it } from "vitest";
import { layoutWorkflow } from "./layout";
import { sampleWorkflow } from "../test/samples";

describe("layoutWorkflow", () => {
  it("creates one node per state with display data", () => {
    const { nodes } = layoutWorkflow(sampleWorkflow());
    expect(nodes.map((n) => n.id)).toEqual(["new", "screening", "hired", "rejected"]);
    expect(nodes[0]).toMatchObject({ type: "state", data: { label: "New", stateKey: "new", color: "gray", initial: true, terminal: false } });
    expect(nodes[2].data).toMatchObject({ terminal: true, initial: false, color: "green" });
  });

  it("lays states out left to right in transition order", () => {
    const { nodes } = layoutWorkflow(sampleWorkflow());
    const x = Object.fromEntries(nodes.map((n) => [n.id, n.position.x]));
    expect(x.new).toBeLessThan(x.screening);
    expect(x.screening).toBeLessThan(x.hired);
  });

  it("creates one edge per transition source, labelled and indexed", () => {
    const { edges } = layoutWorkflow(sampleWorkflow());
    expect(edges).toHaveLength(4);
    const reject = edges.filter((e) => e.data?.transitionIndex === 2);
    expect(reject.map((e) => [e.source, e.target])).toEqual([
      ["new", "rejected"],
      ["screening", "rejected"],
    ]);
    expect(reject[0].label).toBe("Reject");
    expect(new Set(edges.map((e) => e.id)).size).toBe(4);
  });

  it("skips edges that reference unknown states and ignores duplicate state keys", () => {
    const wf = sampleWorkflow();
    wf.states.push({ key: "new", label: "Duplicate" });
    wf.transitions.push({ key: "ghost", label: "Ghost", from: ["nowhere"], to: "new", guard: {} });
    const { nodes, edges } = layoutWorkflow(wf);
    expect(nodes).toHaveLength(4);
    expect(edges).toHaveLength(4);
  });

  it("marks the selected state and transition", () => {
    expect(layoutWorkflow(sampleWorkflow(), { kind: "state", index: 1 }).nodes[1].selected).toBe(true);
    const { edges } = layoutWorkflow(sampleWorkflow(), { kind: "transition", index: 2 });
    expect(edges.filter((e) => e.selected)).toHaveLength(2);
  });

  it("handles an empty workflow", () => {
    expect(layoutWorkflow({ ...sampleWorkflow(), states: [], transitions: [] })).toEqual({ nodes: [], edges: [] });
  });
});
