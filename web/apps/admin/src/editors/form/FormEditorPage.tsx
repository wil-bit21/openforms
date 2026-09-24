import { useEffect, useMemo, useReducer, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { OpenFormsError } from "@openforms/sdk";
import { editorKeys, listWorkflowSlugs, loadForm } from "../api";
import { CodeManagedBanner } from "../shared/CodeManagedBanner";
import { EditorLayout, type EditorTab } from "../shared/EditorLayout";
import { indexFromPath, mergeProblems } from "../shared/problems";
import { useDefinitionSave } from "../shared/useDefinitionSave";
import { useUnsavedChangesGuard } from "../shared/useUnsavedChangesGuard";
import { validateForm } from "../shared/validation";
import { YamlPane } from "../shared/YamlPane";
import type { FormDef, Problem, Source } from "../shared/types";
import { FieldInspector } from "./FieldInspector";
import { FieldList } from "./FieldList";
import { FormMetaPanel } from "./FormMetaPanel";
import { FormPreview } from "./FormPreview";
import { formEditorReducer, initFormEditor, newFormDefinition } from "./formReducer";
import "./form-editor.css";

export function FormEditorPage() {
  const { slug } = useParams<{ slug: string }>();
  const loaded = useQuery({
    queryKey: editorKeys.definition("form", slug ?? ""),
    queryFn: () => loadForm(slug!),
    enabled: Boolean(slug),
  });
  const workflows = useQuery({ queryKey: editorKeys.workflowSlugs, queryFn: listWorkflowSlugs });

  if (slug && loaded.isPending) return <p className="of-ed-hint">Loading form…</p>;
  if (slug && loaded.isError) {
    const notFound = loaded.error instanceof OpenFormsError && loaded.error.status === 404;
    return (
      <p role="alert" className="of-ed-alert">
        {notFound ? `Form "${slug}" was not found.` : `Could not load the form: ${loaded.error.message}`}
      </p>
    );
  }
  return (
    <FormEditor
      key={slug ?? "new"}
      initial={loaded.data?.definition ?? newFormDefinition()}
      source={loaded.data?.source}
      isNew={!slug}
      workflowSlugs={workflows.data ?? []}
    />
  );
}

function FormEditor({
  initial,
  source,
  isNew,
  workflowSlugs,
}: {
  initial: FormDef;
  source?: Source;
  isNew: boolean;
  workflowSlugs: string[];
}) {
  const navigate = useNavigate();
  const [state, dispatch] = useReducer(formEditorReducer, initial, initFormEditor);
  const [tab, setTab] = useState<EditorTab>("visual");
  const [blocked, setBlocked] = useState<string | null>(null);
  const { save, saving, serverProblems, error, clearServerProblems } = useDefinitionSave("form");

  const clientProblems = useMemo(() => validateForm(state.def), [state.def]);
  const problems = useMemo(() => mergeProblems(clientProblems, serverProblems), [clientProblems, serverProblems]);

  // Server problems describe the last save attempt; drop them once the user edits again.
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
      navigate(`/forms/${slug}`);
    });
  };

  const onSelectProblem = (problem: Problem) => {
    setTab("visual");
    dispatch({ type: "select", index: indexFromPath(problem.path, "fields") });
  };

  return (
    <EditorLayout
      title={isNew ? "New form" : `Edit form: ${state.def.title || state.def.slug}`}
      tab={tab}
      onTabChange={setTab}
      onSave={onSave}
      saving={saving}
      dirty={state.dirty}
      banner={<CodeManagedBanner kind="form" source={source} />}
      problems={problems}
      onSelectProblem={onSelectProblem}
      error={error}
      saveBlockedMessage={blocked}
    >
      {tab === "visual" ? (
        <div className="of-fe-grid">
          <FieldList fields={state.def.fields} selected={state.selected} problems={problems} dispatch={dispatch} />
          {state.selected === null ? (
            <FormMetaPanel def={state.def} isNew={isNew} workflowSlugs={workflowSlugs} problems={problems} dispatch={dispatch} />
          ) : (
            <FieldInspector def={state.def} index={state.selected} problems={problems} dispatch={dispatch} />
          )}
          <FormPreview def={state.def} />
        </div>
      ) : (
        <YamlPane value={state.def} onApply={(next) => dispatch({ type: "replace", def: next as unknown as FormDef })} />
      )}
    </EditorLayout>
  );
}
