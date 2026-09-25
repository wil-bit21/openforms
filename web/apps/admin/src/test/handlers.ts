import { http, HttpResponse } from "msw";
import * as f from "./fixtures";

export const API = `${window.location.origin}/api/v1`;
export const api = (path: string) => `${API}${path}`;

export function apiError(status: number, code: string, message: string, details?: { path: string; message: string }[]) {
  return HttpResponse.json({ error: { code, message, ...(details ? { details } : {}) } }, { status });
}

/** Happy-path defaults: an admin is signed in and one form/workflow/submission exists. */
export const defaultHandlers = [
  http.get(api("/auth/me"), () => HttpResponse.json({ principal: f.makePrincipal() })),
  http.post(api("/auth/logout"), () => new HttpResponse(null, { status: 204 })),
  http.get(api("/forms"), () => HttpResponse.json({ items: [f.makeFormSummary()] })),
  http.get(api("/forms/:slug"), ({ params }) =>
    params.slug === "job-application"
      ? HttpResponse.json({ form: f.makeFormRecord() })
      : apiError(404, "not_found", "form not found")),
  http.get(api("/forms/:slug/versions"), () =>
    HttpResponse.json({ items: [f.makeVersionInfo(), f.makeVersionInfo({ version: 2, source: "ui", createdBy: "admin@example.com" })] })),
  http.get(api("/workflows"), () => HttpResponse.json({ items: [f.makeWorkflowSummary()] })),
  http.get(api("/workflows/:slug"), ({ params }) =>
    params.slug === "hiring"
      ? HttpResponse.json({ workflow: f.makeWorkflowRecord() })
      : apiError(404, "not_found", "workflow not found")),
  http.get(api("/workflows/:slug/versions"), () =>
    HttpResponse.json({ items: [f.makeVersionInfo({ version: 2 }), f.makeVersionInfo({ version: 1, source: "seed", createdBy: "" })] })),
  http.get(api("/submissions"), () => HttpResponse.json({ items: [f.makeSubmission()], nextCursor: null })),
  http.get(api("/users"), () => HttpResponse.json({ items: [
    f.makeUser({ id: ids().admin, email: "admin@example.com", name: "Ada Admin", roles: ["admin"] }),
    f.makeUser(),
  ] })),
];

function ids() {
  return f.ids;
}
