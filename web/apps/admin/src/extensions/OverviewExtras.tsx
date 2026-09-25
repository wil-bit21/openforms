import { VersionHistory } from "../editors/history/VersionHistory";

// Extension point defined by Plan 07; Plan 08 renders the version history here.
export function OverviewExtras({ kind, slug }: { kind: "form" | "workflow"; slug: string }): JSX.Element | null {
  return <VersionHistory kind={kind} slug={slug} />;
}
