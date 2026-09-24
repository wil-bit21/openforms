import type { ReactNode } from "react";
import { ApiKeysPage } from "./pages/ApiKeysPage";
import { FormOverviewPage } from "./pages/FormOverviewPage";
import { FormsPage } from "./pages/FormsPage";
import { InboxPage } from "./pages/InboxPage";
import { JobsPage } from "./pages/JobsPage";
import { SubmissionPage } from "./pages/SubmissionPage";
import { UsersPage } from "./pages/UsersPage";
import { WorkflowOverviewPage } from "./pages/WorkflowOverviewPage";
import { WorkflowsPage } from "./pages/WorkflowsPage";

/** A page inside the authenticated shell. `path` is relative to the /admin basename. */
export type AdminRoute = { path: string; element: ReactNode; adminOnly?: boolean };

/**
 * Extension point: Plan 08 appends editor routes to this array
 * ("forms/new", "forms/:slug/edit", "workflows/new", "workflows/:slug/edit", all adminOnly).
 */
export const routes: AdminRoute[] = [
  { path: "submissions", element: <InboxPage /> },
  { path: "submissions/:id", element: <SubmissionPage /> },
  { path: "forms", element: <FormsPage /> },
  { path: "forms/:slug", element: <FormOverviewPage /> },
  { path: "workflows", element: <WorkflowsPage /> },
  { path: "workflows/:slug", element: <WorkflowOverviewPage /> },
  { path: "users", element: <UsersPage />, adminOnly: true },
  { path: "api-keys", element: <ApiKeysPage />, adminOnly: true },
  { path: "jobs", element: <JobsPage />, adminOnly: true },
];
