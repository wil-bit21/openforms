export type NavItem = { to: string; label: string; adminOnly?: boolean };

export const navItems: NavItem[] = [
  { to: "/submissions", label: "Inbox" },
  { to: "/forms", label: "Forms" },
  { to: "/workflows", label: "Workflows" },
  { to: "/users", label: "Users", adminOnly: true },
  { to: "/api-keys", label: "API keys", adminOnly: true },
  { to: "/jobs", label: "Jobs", adminOnly: true },
];
