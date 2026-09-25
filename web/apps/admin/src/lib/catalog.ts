import { useQueries, useQuery } from "@tanstack/react-query";
import type { FormDefinition, FormSummary, WorkflowDefinition } from "@openforms/sdk";
import { client } from "../api";
import { qk } from "../queryKeys";

export type Catalog = {
  forms: FormSummary[];
  formTitle(slug: string): string;
  formDef(slug: string): FormDefinition | undefined;
  workflowDef(formSlug: string): WorkflowDefinition | undefined;
  stateColor(formSlug: string, state: string): string | undefined;
};

/** Current form and workflow definitions, used to label and color inbox rows. Definitions are cached for a minute. */
export function useCatalog(): Catalog {
  const formsQuery = useQuery({ queryKey: qk.forms, queryFn: () => client.listForms() });
  const forms = formsQuery.data ?? [];

  const formQueries = useQueries({
    queries: forms.map((f) => ({ queryKey: qk.form(f.slug), queryFn: () => client.getForm(f.slug), staleTime: 60_000 })),
  });
  const workflowSlugs = [...new Set(forms.map((f) => f.workflow).filter((s): s is string => !!s))];
  const workflowQueries = useQueries({
    queries: workflowSlugs.map((slug) => ({ queryKey: qk.workflow(slug), queryFn: () => client.getWorkflow(slug), staleTime: 60_000 })),
  });

  const formDefs = new Map<string, FormDefinition>();
  for (const q of formQueries) if (q.data) formDefs.set(q.data.slug, q.data.definition);
  const workflowDefs = new Map<string, WorkflowDefinition>();
  for (const q of workflowQueries) if (q.data) workflowDefs.set(q.data.slug, q.data.definition);

  const workflowDef = (formSlug: string) => {
    const slug = forms.find((f) => f.slug === formSlug)?.workflow;
    return slug ? workflowDefs.get(slug) : undefined;
  };

  return {
    forms,
    formTitle: (slug) => forms.find((f) => f.slug === slug)?.title ?? slug,
    formDef: (slug) => formDefs.get(slug),
    workflowDef,
    stateColor: (formSlug, state) => workflowDef(formSlug)?.states.find((s) => s.key === state)?.color,
  };
}
