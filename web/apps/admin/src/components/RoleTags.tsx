export function RoleTags({ roles }: { roles: string[] }) {
  if (!roles.length) return <span className="muted">No roles</span>;
  return (
    <span className="tags">
      {roles.map((r) => (
        <span key={r} className="tag">{r}</span>
      ))}
    </span>
  );
}
