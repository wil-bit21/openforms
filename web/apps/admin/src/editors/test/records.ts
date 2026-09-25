import { makeFormRecord, makeWorkflowRecord } from "../../test/fixtures";
import type { DefinitionKind, FormDef, WorkflowDef } from "../shared/types";
import { sampleForm, sampleWorkflow } from "./samples";

// Records exactly as the API returns them (spec §7.2), built on Plan 07's factories.
export function formRecordJson(def: FormDef = sampleForm(), extra: { source?: string; version?: number } = {}) {
  return makeFormRecord({
    slug: def.slug,
    version: extra.version ?? 3,
    source: (extra.source ?? "ui") as never,
    definition: def as never,
  });
}

export function workflowRecordJson(def: WorkflowDef = sampleWorkflow(), extra: { source?: string; version?: number } = {}) {
  return makeWorkflowRecord({
    slug: def.slug,
    version: extra.version ?? 3,
    source: (extra.source ?? "ui") as never,
    definition: def as never,
  });
}

export function versionList(versions: number[], source = "ui") {
  return versions.map((v) => ({
    version: v,
    hash: `hash${v}`,
    source,
    createdBy: "admin@demo.local",
    createdAt: `2026-09-${String(10 + v).padStart(2, "0")}T10:00:00Z`,
  }));
}

export function applyItem(kind: DefinitionKind, slug: string, version: number, changed = true) {
  return { kind, slug, version, changed, created: false };
}
