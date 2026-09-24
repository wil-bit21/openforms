import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { editorKeys, invalidateDefinition, listVersions, loadVersion, saveDefinition, useIsAdmin } from "../api";
import { toYaml } from "../shared/yaml";
import { SOURCE_LABELS, type DefinitionKind } from "../shared/types";
import { DiffView } from "./DiffView";
import "../shared/editors.css";
import "./history.css";

export function VersionHistory({ kind, slug }: { kind: DefinitionKind; slug: string }) {
  const qc = useQueryClient();
  const isAdmin = useIsAdmin();
  const [picked, setPicked] = useState<number[] | null>(null);
  const [status, setStatus] = useState<string | null>(null);

  const versions = useQuery({ queryKey: editorKeys.versions(kind, slug), queryFn: () => listVersions(kind, slug) });
  const list = versions.data ?? [];
  const selection = picked ?? list.slice(0, 2).map((v) => v.version);
  const older = selection.length === 2 ? Math.min(...selection) : undefined;
  const newer = selection.length === 2 ? Math.max(...selection) : undefined;

  // Versions are immutable (spec §5.5), so they never go stale.
  const olderQuery = useQuery({
    queryKey: editorKeys.version(kind, slug, older ?? 0),
    queryFn: () => loadVersion(kind, slug, older!),
    enabled: older !== undefined,
    staleTime: Infinity,
  });
  const newerQuery = useQuery({
    queryKey: editorKeys.version(kind, slug, newer ?? 0),
    queryFn: () => loadVersion(kind, slug, newer!),
    enabled: newer !== undefined,
    staleTime: Infinity,
  });

  const restore = useMutation({
    mutationFn: async (version: number) => {
      const def = await qc.fetchQuery({
        queryKey: editorKeys.version(kind, slug, version),
        queryFn: () => loadVersion(kind, slug, version),
        staleTime: Infinity,
      });
      return saveDefinition(kind, def);
    },
    onSuccess: async (result) => {
      setStatus(result.changed ? `Restored as version ${result.version}.` : "That version matches the current one, so nothing changed.");
      setPicked(null);
      await invalidateDefinition(qc, kind, slug);
    },
    onError: (err) => setStatus(`Restore failed: ${err instanceof Error ? err.message : String(err)}`),
  });

  const toggle = (version: number, on: boolean) =>
    setPicked((prev) => {
      const current = prev ?? selection;
      if (!on) return current.filter((v) => v !== version);
      const next = [...current.filter((v) => v !== version), version];
      return next.length > 2 ? next.slice(next.length - 2) : next;
    });

  if (versions.isPending) return <p className="of-ed-hint">Loading version history…</p>;
  if (versions.isError) {
    return (
      <p role="alert" className="of-ed-alert">
        Could not load version history: {versions.error.message}
      </p>
    );
  }

  return (
    <section className="of-vh" aria-labelledby={`vh-${kind}-${slug}`}>
      <h2 id={`vh-${kind}-${slug}`}>Version history</h2>
      {status ? <p role="status">{status}</p> : null}
      <table className="of-vh-versions">
        <caption className="sr-only">Versions of {slug}</caption>
        <thead>
          <tr>
            <th>Compare</th>
            <th>Version</th>
            <th>Source</th>
            <th>By</th>
            <th>Created</th>
            {isAdmin ? (
              <th>
                <span className="sr-only">Actions</span>
              </th>
            ) : null}
          </tr>
        </thead>
        <tbody>
          {list.map((v, i) => (
            <tr key={v.version}>
              <td>
                <input
                  type="checkbox"
                  aria-label={`Compare version ${v.version}`}
                  checked={selection.includes(v.version)}
                  onChange={(e) => toggle(v.version, e.target.checked)}
                />
              </td>
              <td>
                v{v.version}
                {i === 0 ? <span className="of-vh-current"> current</span> : null}
              </td>
              <td>
                <span className={`of-vh-source of-vh-source--${v.source}`}>{SOURCE_LABELS[v.source] ?? v.source}</span>
              </td>
              <td>{v.createdBy || "—"}</td>
              <td>
                <time dateTime={v.createdAt}>{new Date(v.createdAt).toLocaleString()}</time>
              </td>
              {isAdmin ? (
                <td>
                  {i > 0 ? (
                    <button
                      type="button"
                      className="of-ed-btn"
                      disabled={restore.isPending}
                      onClick={() => {
                        if (window.confirm(`Restore version ${v.version}? Its content will be saved as a new version.`)) restore.mutate(v.version);
                      }}
                    >
                      Restore version {v.version}
                    </button>
                  ) : null}
                </td>
              ) : null}
            </tr>
          ))}
        </tbody>
      </table>
      {list.length < 2 ? (
        <p className="of-ed-hint">Only one version so far. A new version appears each time the {kind} changes.</p>
      ) : older === undefined || newer === undefined ? (
        <p className="of-ed-hint">Select two versions to compare.</p>
      ) : olderQuery.data && newerQuery.data ? (
        <DiffView leftTitle={`v${older}`} rightTitle={`v${newer}`} left={toYaml(olderQuery.data)} right={toYaml(newerQuery.data)} />
      ) : olderQuery.isError || newerQuery.isError ? (
        <p role="alert" className="of-ed-alert">
          Could not load one of the versions to compare.
        </p>
      ) : (
        <p className="of-ed-hint">Loading comparison…</p>
      )}
    </section>
  );
}
