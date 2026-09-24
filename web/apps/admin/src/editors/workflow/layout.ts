import dagre from "@dagrejs/dagre";
import { MarkerType, type Edge, type Node } from "@xyflow/react";
import type { WorkflowDef } from "../shared/types";
import type { WorkflowSelection } from "./workflowReducer";

export const NODE_WIDTH = 168;
export const NODE_HEIGHT = 56;

export type StateNodeData = { label: string; stateKey: string; color: string; terminal: boolean; initial: boolean };
export type TransitionEdgeData = { transitionIndex: number };

export function layoutWorkflow(
  def: WorkflowDef,
  selection: WorkflowSelection = { kind: "workflow" },
): { nodes: Node<StateNodeData, "state">[]; edges: Edge<TransitionEdgeData>[] } {
  const graph = new dagre.graphlib.Graph({ multigraph: true });
  graph.setGraph({ rankdir: "LR", nodesep: 40, ranksep: 90, marginx: 16, marginy: 16 });
  graph.setDefaultEdgeLabel(() => ({}));

  // dagre crashes on an empty node id (a state key being retyped), so prefix ids internally.
  const gid = (key: string) => `s:${key}`;
  const seen = new Set<string>();
  const states = def.states
    .map((state, index) => ({ state, index }))
    .filter(({ state }) => {
      if (seen.has(state.key)) return false;
      seen.add(state.key);
      return true;
    });
  states.forEach(({ state }) => graph.setNode(gid(state.key), { width: NODE_WIDTH, height: NODE_HEIGHT }));

  const edges: Edge<TransitionEdgeData>[] = [];
  def.transitions.forEach((t, transitionIndex) => {
    t.from.forEach((from) => {
      if (!seen.has(from) || !seen.has(t.to)) return;
      const id = `${t.key}:${from}->${t.to}:${transitionIndex}`;
      graph.setEdge(gid(from), gid(t.to), {}, id);
      edges.push({
        id,
        source: from,
        target: t.to,
        label: t.label || t.key,
        data: { transitionIndex },
        selected: selection.kind === "transition" && selection.index === transitionIndex,
        markerEnd: { type: MarkerType.ArrowClosed },
      });
    });
  });

  if (states.length > 0) dagre.layout(graph);

  const nodes = states.map(({ state, index }) => {
    const positioned = graph.node(gid(state.key));
    return {
      id: state.key,
      type: "state" as const,
      position: { x: positioned.x - NODE_WIDTH / 2, y: positioned.y - NODE_HEIGHT / 2 },
      data: {
        label: state.label || state.key,
        stateKey: state.key,
        color: state.color ?? "gray",
        terminal: Boolean(state.terminal),
        initial: def.initial === state.key,
      },
      selected: selection.kind === "state" && selection.index === index,
    };
  });

  return { nodes, edges };
}
