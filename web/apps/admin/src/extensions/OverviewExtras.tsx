/**
 * Extension point rendered at the bottom of the form and workflow overview pages.
 * Plan 07 renders nothing; Plan 08 replaces this body with the version-history panel.
 */
export function OverviewExtras(_props: { kind: "form" | "workflow"; slug: string }): JSX.Element | null {
  return null;
}
