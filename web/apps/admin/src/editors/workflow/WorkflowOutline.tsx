import type { Dispatch } from "react";
import { problemsUnder } from "../shared/problems";
import type { Problem, WorkflowDef } from "../shared/types";
import type { WorkflowEditorAction, WorkflowSelection } from "./workflowReducer";

function Badge({ problems, prefix }: { problems: Problem[]; prefix: string }) {
  const n = problemsUnder(problems, prefix).length;
  if (n === 0) return null;
  return (
    <span className="of-ed-badge" aria-label={n === 1 ? "1 problem" : `${n} problems`}>
      {n}
    </span>
  );
}

export function WorkflowOutline({
  def,
  selection,
  problems,
  dispatch,
}: {
  def: WorkflowDef;
  selection: WorkflowSelection;
  problems: Problem[];
  dispatch: Dispatch<WorkflowEditorAction>;
}) {
  const current = (kind: WorkflowSelection["kind"], index?: number) =>
    selection.kind === kind && (index === undefined || ("index" in selection && selection.index === index)) ? "true" : undefined;
  const select = (next: WorkflowSelection) => dispatch({ type: "select", selection: next });
  const settingsProblems = problems.filter((p) => !/^(states|transitions|fields)\[/.test(p.path)).length;

  return (
    <nav className="of-ed-list" aria-label="Workflow outline">
      <button type="button" className="of-ed-list__item" aria-current={current("workflow")} onClick={() => select({ kind: "workflow" })}>
        <span>
          Workflow settings
          {settingsProblems > 0 ? (
            <span className="of-ed-badge" aria-label={settingsProblems === 1 ? "1 problem" : `${settingsProblems} problems`}>
              {settingsProblems}
            </span>
          ) : null}
        </span>
      </button>

      <h3>States</h3>
      <ul>
        {def.states.map((s, i) => (
          <li key={i}>
            <button type="button" className="of-ed-list__item" aria-current={current("state", i)} onClick={() => select({ kind: "state", index: i })}>
              <span>
                <span className={`of-wf-dot of-wf-dot--${s.color ?? "gray"}`} aria-hidden="true" />
                {s.label || s.key}
                <Badge problems={problems} prefix={`states[${i}]`} />
              </span>
              <small>
                {s.key}
                {def.initial === s.key ? " · initial" : ""}
                {s.terminal ? " · terminal" : ""}
              </small>
            </button>
          </li>
        ))}
      </ul>
      <button type="button" className="of-ed-btn" onClick={() => dispatch({ type: "addState" })}>
        Add state
      </button>

      <h3>Transitions</h3>
      <ul>
        {def.transitions.map((t, i) => (
          <li key={i}>
            <button type="button" className="of-ed-list__item" aria-current={current("transition", i)} onClick={() => select({ kind: "transition", index: i })}>
              <span>
                {t.label || t.key}
                <Badge problems={problems} prefix={`transitions[${i}]`} />
              </span>
              <small>
                {t.from.join(", ")} → {t.to}
              </small>
            </button>
          </li>
        ))}
      </ul>
      <button type="button" className="of-ed-btn" onClick={() => dispatch({ type: "addTransition" })} disabled={def.states.length === 0}>
        Add transition
      </button>

      <h3>Workflow fields</h3>
      <ul>
        {(def.fields ?? []).map((f, i) => (
          <li key={i}>
            <button type="button" className="of-ed-list__item" aria-current={current("field", i)} onClick={() => select({ kind: "field", index: i })}>
              <span>
                {f.label || f.key}
                <Badge problems={problems} prefix={`fields[${i}]`} />
              </span>
              <small>{f.key}</small>
            </button>
          </li>
        ))}
      </ul>
      <button type="button" className="of-ed-btn" onClick={() => dispatch({ type: "addField" })}>
        Add field
      </button>
    </nav>
  );
}
