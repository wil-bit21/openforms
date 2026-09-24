# openforms web workspace

pnpm workspace for everything that runs in a browser.

| Path | Package | What it is |
|---|---|---|
| `packages/sdk` | `@openforms/sdk` | Typed API client, definition types generated from `schemas/`, shared submission logic |
| `packages/react` | `@openforms/react` | `useOpenForm`, `<OpenForm>`, `<StatusTracker>`, `styles.css` |
| `packages/embed` | `@openforms/embed` | `embed.js` — `<div data-openforms="slug">` → auto-resizing iframe |
| `apps/hosted` | `@openforms/hosted` | Hosted pages served by the Go binary at `/f/:slug` and `/s/:id` |

## Commands (from the repo root)

```bash
pnpm -C web install
pnpm -C web test          # all package tests (Vitest)
pnpm -C web test:scripts  # build-script tests (node:test)
pnpm -C web typecheck
pnpm -C web build         # builds packages, then apps, then copies into internal/webui/dist
pnpm -C web/packages/sdk gen   # regenerate definition types after editing schemas/*.schema.json
```

## Developing the hosted pages

Run the Go server on :8080 (`go run ./cmd/openforms serve`), then:

```bash
pnpm -C web/apps/hosted dev
# open http://localhost:5174/_app/hosted/f/<slug>
```

The Vite dev server proxies `/api` and `/healthz` to `http://localhost:8080`.

## Embedding a form

```html
<div data-openforms="contact"></div>
<script src="https://forms.example.com/embed.js" async></script>
<script>
  document.addEventListener("openforms:submitted", (e) => console.log("submitted", e.detail));
</script>
```

Call `window.OpenForms.scan()` after inserting new `data-openforms` elements in a single-page app.
