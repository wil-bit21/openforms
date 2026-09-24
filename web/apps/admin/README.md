# @openforms/admin

The openforms admin SPA, served by the Go binary at `/admin/*` (assets under `/_app/admin/`).

## Develop

```bash
docker compose up -d postgres mailpit
go run ./cmd/openforms serve            # API on :8080
pnpm -C web/apps/admin dev              # http://localhost:5174/_app/admin/ (proxies /api → :8080)
pnpm -C web/apps/admin test             # Vitest + Testing Library + MSW
pnpm -C web/apps/admin typecheck
```

## Extension points (used by Plan 08)

| What | Where | How to extend |
|---|---|---|
| Pages | `src/routes.tsx` → `AdminRoute = { path; element; adminOnly? }`, `routes: AdminRoute[]` | Append an entry; paths are relative to `/admin` (e.g. `"forms/:slug/edit"`). `adminOnly: true` wraps the page in `<RequireAdmin>`. |
| Sidebar | `src/nav.ts` → `NavItem = { to; label; adminOnly? }`, `navItems` | Append an entry. |
| Overview panels | `src/extensions/OverviewExtras.tsx` → `OverviewExtras({ kind: "form" \| "workflow"; slug })` | Replace the body; it renders at the bottom of `forms/:slug` and `workflows/:slug`. |
| API | `src/api.ts` → `client` (`OpenFormsClient`), `src/queryKeys.ts` → `qk` | Import directly; invalidate with `qk.*`. |
| Tests | `src/test/render.tsx` → `renderWithProviders(ui, { route?, path? })`, `renderApp(route)`; `src/test/server.ts` → MSW `server`; `src/test/handlers.ts` → `api()`, `apiError()`; `src/test/fixtures.ts` → `make*` factories | Use `server.use(...)` per test for extra endpoints. |

Existing links already point at Plan 08 routes: "New form" → `/admin/forms/new`, "Edit" → `/admin/forms/:slug/edit`, "New workflow" → `/admin/workflows/new`, "Edit" → `/admin/workflows/:slug/edit` (admins only).
