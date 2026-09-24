// Minimal stand-in for @xyflow/react in jsdom (no layout engine, no ResizeObserver).
// Usage in a test file: vi.mock("@xyflow/react", () => import("../test/xyflowMock"));
import type { ComponentType, MouseEvent, ReactNode } from "react";

type NodeLike = { id: string; type?: string; data: Record<string, unknown>; selected?: boolean };
type EdgeLike = { id: string; source: string; target: string; label?: ReactNode; data?: Record<string, unknown> };
type NodeComponent = ComponentType<{ id: string; data: Record<string, unknown>; selected: boolean }>;

export function ReactFlow(props: {
  nodes: NodeLike[];
  edges: EdgeLike[];
  nodeTypes?: Record<string, NodeComponent>;
  onNodeClick?: (event: MouseEvent, node: NodeLike) => void;
  onEdgeClick?: (event: MouseEvent, edge: EdgeLike) => void;
  children?: ReactNode;
}) {
  return (
    <div data-testid="reactflow">
      {props.nodes.map((n) => {
        const Custom = n.type ? props.nodeTypes?.[n.type] : undefined;
        return (
          <div key={n.id} role="button" tabIndex={0} aria-label={`node ${n.id}`} onClick={(e) => props.onNodeClick?.(e, n)}>
            {Custom ? <Custom id={n.id} data={n.data} selected={Boolean(n.selected)} /> : n.id}
          </div>
        );
      })}
      {props.edges.map((e) => (
        <button key={e.id} type="button" aria-label={`edge ${e.id}`} onClick={(ev) => props.onEdgeClick?.(ev, e)}>
          {e.label}
        </button>
      ))}
      {props.children}
    </div>
  );
}

export const Handle = () => null;
export const Background = () => null;
export const Controls = () => null;
export const Position = { Left: "left", Right: "right", Top: "top", Bottom: "bottom" } as const;
export const MarkerType = { Arrow: "arrow", ArrowClosed: "arrowclosed" } as const;
