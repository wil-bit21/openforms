import { useEffect, useMemo, useReducer, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { OpenFormsError } from "@openforms/sdk";
import { editorKeys, loadWorkflow } from "../api";
import { CodeManagedBanner } from "../shared/CodeManagedBanner";
import { EditorLayout, type EditorTab } from "../shared/EditorLayout";
import { indexFromPath, mergeProblems } from "../shared/problems";
import { useDefinitionSave } from "../shared/useDefinitionSave";
import { useUnsavedChangesGuard } from "../shared/useUnsavedChangesGuard";
import { validateWorkflow } from "../shared/validation";
import { YamlPane } from "../shared/YamlPane";
import type { Problem, Source, WorkflowDef } from "../shared/types";
import { StateInspector } from "./StateInspector";
import { TransitionInspector } from "./TransitionInspector";
import { WorkflowDiagram } from "./WorkflowDiagram";
import { WorkflowFieldInspector } from "./WorkflowFieldInspector";
import { WorkflowMetaPanel } from "./WorkflowMetaPanel";
import { WorkflowOutline } from "./WorkflowOutline";
import { initWorkflowEditor, newWorkflowDefinition, workflowEditorReducer, type WorkflowSelection } from "./workflowReducer";
import "./workflow-editor.css";

export function WorkflowEditorPage() {
  const { slug } = useParams<{ slug: string }>();
  const loaded = useQuery({
    queryKey: editorKeys.definition("workflow", slug ?? ""),
    queryFn: () => loadWorkflow(slug!),
    enabled: Boolean(slug),
  });

  if (slug && loaded.isPending) return <p className="of-ed-hint">Loading workflow…</p>;
  if (slug && loaded.isError) {
    const notFound = loaded.error instanceof OpenFormsError && loaded.error.status === 404;
    return (
      <p role="alert" className="of-ed-alert">
        {notFound ? `Workflow "${slug}" was not found.` : `Could not load the workflow: ${loaded.error.message}`}
      </p>
    );
  }
  return (
    <WorkflowEditor
      key={slug ?? "new"}
      initial={loaded.data?.definition ?? newWorkflowDefinition()}
      source={loaded.data?.source}
      isNew={!slug}
    />
  );
}

function selectionForProblem(problem: Problem): WorkflowSelection {
  const state = indexFromPath(problem.path, "states");
  if (state !== null) return { kind: "state", index: state };
  const transition = indexFromPath(problem.path, "transitions");
  if (transition !== null) return { kind: "transition", index: transition };
  const field = indexFromPath(problem.path, "fields");
  if (field !== null) return { kind: "field", index: field };
  return { kind: "workflow" };
}

function WorkflowEditor({ initial, source, isNew }: { initial: WorkflowDef; source?: Source; isNew: boolean }) {
  const navigate = useNavigate();
  const [state, dispatch] = useReducer(workflowEditorReducer, initial, initWorkflowEditor);
  const [tab, setTab] = useState<EditorTab>("visual");
  const [blocked, setBlocked] = useState<string | null>(null);
  const { save, saving, serverProblems, error, clearServerProblems } = useDefinitionSave("workflow");

  const clientProblems = useMemo(() => validateWorkflow(state.def), [state.def]);
  const problems = useMemo(() => mergeProblems(clientProblems, serverProblems), [clientProblems, serverProblems]);

  useEffect(() => {
    clearServerProblems();
    setBlocked(null);
  }, [state.def, clearServerProblems]);

  useUnsavedChangesGuard(state.dirty);

  const onSave = () => {
    if (clientProblems.length > 0) {
      const n = clientProblems.length;
      setBlocked(`Fix ${n} ${n === 1 ? "problem" : "problems"} before saving.`);
      return;
    }
    const slug = state.def.slug;
    save(state.def, isNew, () => {
      dispatch({ type: "markSaved" });
      navigate(`/workflows/${slug}`);
    });
  };

  const select = (selection: WorkflowSelection) => dispatch({ type: "select", selection });
  const sel = state.selection;

  return (
    <EditorLayout
      title={isNew ? "New workflow" : `Edit workflow: ${state.def.title || state.def.slug}`}
      tab={tab}
      onTabChange={setTab}
      onSave={onSave}
      saving={saving}
      dirty={state.dirty}
      banner={<CodeManagedBanner kind="workflow" source={source} />}
      problems={problems}
      onSelectProblem={(p) => {
        setTab("visual");
        select(selectionForProblem(p));
      }}
      error={error}
      saveBlockedMessage={blocked}
    >
      {tab === "visual" ? (
        <div className="of-we-grid">
          <WorkflowOutline def={state.def} selection={sel} problems={problems} dispatch={dispatch} />
          <WorkflowDiagram def={state.def} selection={sel} onSelect={select} />
          <div>
            {sel.kind === "workflow" ? <WorkflowMetaPanel def={state.def} isNew={isNew} problems={problems} dispatch={dispatch} /> : null}
            {sel.kind === "state" ? <StateInspector def={state.def} index={sel.index} problems={problems} dispatch={dispatch} /> : null}
            {sel.kind === "transition" ? <TransitionInspector def={state.def} index={sel.index} problems={problems} dispatch={dispatch} /> : null}
            {sel.kind === "field" ? <WorkflowFieldInspector def={state.def} index={sel.index} problems={problems} dispatch={dispatch} /> : null}
          </div>
        </div>
      ) : (
        <YamlPane value={state.def} onApply={(next) => dispatch({ type: "replace", def: next as unknown as WorkflowDef })} />
      )}
    </EditorLayout>
  );
}
