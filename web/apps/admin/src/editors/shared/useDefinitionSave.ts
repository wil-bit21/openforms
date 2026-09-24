import { useCallback, useState } from "react";
import { OpenFormsError } from "@openforms/sdk";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { definitionExists, invalidateDefinition, saveDefinition } from "../api";
import type { DefinitionKind, FormDef, Problem, WorkflowDef } from "./types";

export class SlugTakenError extends Error {}

type SaveResult = { version: number; changed: boolean };

export function useDefinitionSave(kind: DefinitionKind) {
  const qc = useQueryClient();
  const [serverProblems, setServerProblems] = useState<Problem[]>([]);
  const [error, setError] = useState<string | null>(null);

  const { mutate, isPending } = useMutation({
    mutationFn: async ({ def, isNew }: { def: FormDef | WorkflowDef; isNew: boolean }): Promise<SaveResult> => {
      // PUT is an upsert, so "new" must not silently overwrite an existing definition.
      if (isNew && (await definitionExists(kind, def.slug))) {
        throw new SlugTakenError(`A ${kind} with the slug "${def.slug}" already exists. Choose another slug.`);
      }
      return saveDefinition(kind, def);
    },
    onMutate: () => {
      setServerProblems([]);
      setError(null);
    },
    onSuccess: (_result, { def }) => invalidateDefinition(qc, kind, def.slug),
    onError: (err) => {
      if (err instanceof SlugTakenError) {
        setServerProblems([{ path: "slug", message: err.message }]);
      } else if (err instanceof OpenFormsError && err.status === 422) {
        setServerProblems(err.details?.length ? err.details : [{ path: "", message: err.message }]);
      } else {
        setError(err instanceof Error ? err.message : String(err));
      }
    },
  });

  const save = useCallback(
    (def: FormDef | WorkflowDef, isNew: boolean, onSaved: (result: SaveResult) => void) =>
      mutate({ def, isNew }, { onSuccess: (result) => onSaved(result) }),
    [mutate],
  );
  const clearServerProblems = useCallback(() => setServerProblems([]), []);

  return { save, saving: isPending, serverProblems, error, clearServerProblems };
}
