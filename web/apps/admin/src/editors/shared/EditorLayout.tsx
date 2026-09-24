import type { ReactNode } from "react";
import { ProblemList } from "./ProblemList";
import type { Problem } from "./types";
import "./editors.css";

export type EditorTab = "visual" | "yaml";

export function EditorLayout(props: {
  title: string;
  tab: EditorTab;
  onTabChange: (tab: EditorTab) => void;
  onSave: () => void;
  saving: boolean;
  dirty: boolean;
  banner?: ReactNode;
  problems: Problem[];
  onSelectProblem?: (problem: Problem) => void;
  error?: string | null;
  saveBlockedMessage?: string | null;
  children: ReactNode;
}) {
  return (
    <div className="of-ed">
      <header className="of-ed-header">
        <h1>{props.title}</h1>
        {props.dirty ? <span className="of-ed-dirty">Unsaved changes</span> : null}
        <div className="of-ed-tabs" role="tablist" aria-label="Editor mode">
          <button type="button" role="tab" aria-selected={props.tab === "visual"} onClick={() => props.onTabChange("visual")}>
            Visual
          </button>
          <button type="button" role="tab" aria-selected={props.tab === "yaml"} onClick={() => props.onTabChange("yaml")}>
            YAML
          </button>
        </div>
        <button type="button" className="of-ed-btn of-ed-btn--primary" onClick={props.onSave} disabled={props.saving}>
          {props.saving ? "Saving…" : "Save"}
        </button>
      </header>
      {props.banner}
      {props.error ? (
        <p role="alert" className="of-ed-alert">
          Could not save: {props.error}
        </p>
      ) : null}
      {props.saveBlockedMessage ? (
        <p role="alert" className="of-ed-alert">
          {props.saveBlockedMessage}
        </p>
      ) : null}
      <ProblemList problems={props.problems} onSelect={props.onSelectProblem} />
      <div className="of-ed-body">{props.children}</div>
    </div>
  );
}
