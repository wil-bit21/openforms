const COLORS = ["gray", "blue", "green", "yellow", "red", "purple"] as const;
export type BadgeColor = (typeof COLORS)[number];

export function badgeColor(color?: string | null): BadgeColor {
  return (COLORS as readonly string[]).includes(color ?? "") ? (color as BadgeColor) : "gray";
}

export function StateBadge({ label, color }: { label: string; color?: string | null }) {
  const c = badgeColor(color);
  return (
    <span className={`badge badge-${c}`} data-color={c}>
      {label}
    </span>
  );
}
