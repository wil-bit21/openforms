import type { DefinitionKind, Source } from "./types";

export function CodeManagedBanner({ kind, source }: { kind: DefinitionKind; source?: Source }) {
  if (source !== "cli") return null;
  return (
    <div role="note" className="of-ed-banner">
      This {kind} is managed in code. Changes made here will be overwritten by the next <code>openforms push</code>{" "}
      unless you run <code>openforms pull</code>.
    </div>
  );
}
