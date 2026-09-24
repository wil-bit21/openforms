import type { VersionInfo } from "@openforms/sdk";
import { RelativeTime } from "./RelativeTime";

const SOURCE_LABELS: Record<string, string> = { cli: "CLI", ui: "Editor", api: "API", seed: "Seed" };

export function SourceTag({ source }: { source: string }) {
  return <span className="tag" title={`Created via ${source}`}>{SOURCE_LABELS[source] ?? source}</span>;
}

export function VersionTable({ versions }: { versions: VersionInfo[] }) {
  if (versions.length === 0) return <p className="muted">No versions yet.</p>;
  return (
    <table className="table">
      <thead>
        <tr>
          <th>Version</th>
          <th>Source</th>
          <th>By</th>
          <th>Created</th>
          <th>Hash</th>
        </tr>
      </thead>
      <tbody>
        {versions.map((v) => (
          <tr key={v.version}>
            <td>v{v.version}</td>
            <td><SourceTag source={v.source} /></td>
            <td>{v.createdBy || <span className="muted">—</span>}</td>
            <td><RelativeTime iso={v.createdAt} /></td>
            <td className="mono">{v.hash.slice(0, 12)}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
