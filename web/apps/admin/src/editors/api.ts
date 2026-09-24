// The ONLY place the editors touch @openforms/sdk. If Plan 06's method names or
// return shapes differ from the assumptions below, adjust this file only.
import { OpenFormsError, type FormDefinition, type WorkflowDefinition } from "@openforms/sdk";
import { useQuery, type QueryClient } from "@tanstack/react-query";
import { client } from "../api";
import { qk } from "../queryKeys";
import type { DefinitionKind, FormDef, Source, VersionSummary, WorkflowDef } from "./shared/types";

export interface LoadedDefinition<T> {
  definition: T;
  source: Source;
  version: number;
}

export const editorKeys = {
  definition: (kind: DefinitionKind, slug: string) => ["editor", kind, slug] as const,
  versions: (kind: DefinitionKind, slug: string) => ["editor", kind, slug, "versions"] as const,
  version: (kind: DefinitionKind, slug: string, n: number) => ["editor", kind, slug, "version", n] as const,
  workflowSlugs: ["editor", "workflow-slugs"] as const,
};

export async function loadForm(slug: string): Promise<LoadedDefinition<FormDef>> {
  const record = await client.getForm(slug);
  return { definition: record.definition as unknown as FormDef, source: record.source as Source, version: record.version };
}

export async function loadWorkflow(slug: string): Promise<LoadedDefinition<WorkflowDef>> {
  const record = await client.getWorkflow(slug);
  return { definition: record.definition as unknown as WorkflowDef, source: record.source as Source, version: record.version };
}

export async function saveDefinition(
  kind: DefinitionKind,
  def: FormDef | WorkflowDef,
): Promise<{ version: number; changed: boolean }> {
  const item =
    kind === "form"
      ? await client.putForm(def.slug, def as unknown as FormDefinition, { source: "ui" })
      : await client.putWorkflow(def.slug, def as unknown as WorkflowDefinition, { source: "ui" });
  return { version: item.version, changed: item.changed };
}

export async function definitionExists(kind: DefinitionKind, slug: string): Promise<boolean> {
  try {
    if (kind === "form") await client.getForm(slug);
    else await client.getWorkflow(slug);
    return true;
  } catch (err) {
    if (err instanceof OpenFormsError && err.status === 404) return false;
    throw err;
  }
}

export async function listVersions(kind: DefinitionKind, slug: string): Promise<VersionSummary[]> {
  const items = kind === "form" ? await client.formVersions(slug) : await client.workflowVersions(slug);
  return items.map((v) => ({ version: v.version, source: v.source as Source, createdBy: v.createdBy, createdAt: v.createdAt }));
}

export async function loadVersion(kind: DefinitionKind, slug: string, version: number): Promise<FormDef | WorkflowDef> {
  const record = kind === "form" ? await client.formVersion(slug, version) : await client.workflowVersion(slug, version);
  return record.definition as unknown as FormDef | WorkflowDef;
}

export async function listWorkflowSlugs(): Promise<string[]> {
  const items = await client.listWorkflows();
  return items.map((w) => w.slug).sort();
}

export function useIsAdmin(): boolean {
  const { data } = useQuery({ queryKey: qk.me, queryFn: () => client.me() });
  return Boolean(data?.roles?.includes("admin"));
}

export async function invalidateDefinition(qc: QueryClient, kind: DefinitionKind, slug: string): Promise<void> {
  const planKeys =
    kind === "form"
      ? [qk.form(slug), qk.formVersions(slug), qk.forms]
      : [qk.workflow(slug), qk.workflowVersions(slug), qk.workflows];
  await Promise.all([
    qc.invalidateQueries({ queryKey: ["editor", kind, slug] }),
    qc.invalidateQueries({ queryKey: editorKeys.workflowSlugs }),
    ...planKeys.map((queryKey) => qc.invalidateQueries({ queryKey })),
  ]);
}
