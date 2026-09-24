import { useMemo } from "react";
import { Background, Controls, Handle, Position, ReactFlow, type Node, type NodeProps } from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import type { WorkflowDef } from "../shared/types";
import { layoutWorkflow, type StateNodeData, type TransitionEdgeData } from "./layout";
import type { WorkflowSelection } from "./workflowReducer";

export function StateNode({ data, selected }: NodeProps<Node<StateNodeData, "state">>) {
  const className = [
    "of-wf-node",
    `of-wf-node--${data.color}`,
    data.terminal ? "of-wf-node--terminal" : "",
    selected ? "is-selected" : "",
  ]
    .filter(Boolean)
    .join(" ");
  return (
    <div className={className}>
      <Handle type="target" position={Position.Left} />
      {data.initial ? (
        <span className="of-wf-node__initial" aria-label="Initial state" title="Initial state">
          ▶
        </span>
      ) : null}
      <strong>{data.label}</strong>
      <small>{data.stateKey}</small>
      <Handle type="source" position={Position.Right} />
    </div>
  );
}

const nodeTypes = { state: StateNode };

export function WorkflowDiagram({
  def,
  selection,
  onSelect,
}: {
  def: WorkflowDef;
  selection: WorkflowSelection;
  onSelect: (selection: WorkflowSelection) => void;
}) {
  const { nodes, edges } = useMemo(() => layoutWorkflow(def, selection), [def, selection]);
  return (
    <div className="of-wf-diagram" aria-label="Workflow diagram">
      <ReactFlow
        nodes={nodes}
        edges={edges}
        nodeTypes={nodeTypes}
        fitView
        nodesDraggable={false}
        nodesConnectable={false}
        onNodeClick={(_event, node) => {
          const index = def.states.findIndex((s) => s.key === node.id);
          if (index >= 0) onSelect({ kind: "state", index });
        }}
        onEdgeClick={(_event, edge) => {
          const data = edge.data as TransitionEdgeData | undefined;
          if (data) onSelect({ kind: "transition", index: data.transitionIndex });
        }}
      >
        <Background />
        <Controls showInteractive={false} />
      </ReactFlow>
    </div>
  );
}
