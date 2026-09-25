# HTTP API

Base path: `/api/v1`. All bodies are JSON with camelCase keys, and timestamps are RFC 3339 UTC.

## Authentication

| Method | How |
|---|---|
| API key | `Authorization: Bearer ofk_…`. Create keys with `openforms admin create-api-key` or in the admin UI. Keys carry roles, like users. |
| Session | `POST /auth/login` sets the `of_session` cookie (HttpOnly, 30 days). Used by the admin UI. |

Endpoints under `/public/*`, plus `/auth/login`, `/auth/password-reset*` and `/healthz`, need no authentication.

## Errors

```json
{ "error": { "code": "validation_failed", "message": "…", "details": [ { "path": "data.email", "message": "must be a valid email address" } ] } }
```

| Status | `code` | When |
|---|---|---|
| 400 | `bad_request`, `invalid_token` | Malformed JSON or body larger than 1 MiB; an invalid or expired password reset token |
| 401 | `unauthenticated` / `invalid_credentials` | Missing/invalid credentials, wrong password |
| 403 | `forbidden` | Not an admin, or a transition guard rejected your roles |
| 404 | `not_found` | Unknown resource, or a non-public form on a public endpoint |
| 409 | `email_taken`, `invalid_state`, `state_conflict`, `no_workflow` | Conflicts |
| 422 | `validation_failed`, `unknown_transition` | Invalid definitions, submission data or transition input |
| 429 | `rate_limited` | Too many requests (`Retry-After` says how long to wait): public submissions (20/min per IP), sign-in (20 per 5 min per IP, 10 per 15 min per email), password reset (5 per 15 min per IP, 3 per hour per email) |
| 500 | `internal` | Server error (details are logged) |

## Pagination

List endpoints accept `?limit=` (default 50, max 200) and `?cursor=`. Responses have the shape `{"items": [...], "nextCursor": "…" | null}`.

## Public endpoints

| Method | Path | Description |
|---|---|---|
| GET | `/public/config` | `{"demo": bool}` |
| GET | `/public/forms/{slug}` | `{"form": <definition>}` for public forms |
| POST | `/public/forms/{slug}/submissions` | Body `{"data": {...}}` → `201 {"id","state","stateLabel","receiptToken","confirmationMessage"}` |
| GET | `/public/submissions/{id}?token=` | Status for the respondent: `{id, formTitle, state, stateLabel, terminal, states, history, createdAt}` |

```bash
curl -X POST http://localhost:8080/api/v1/public/forms/contact/submissions \
  -H 'Content-Type: application/json' \
  -d '{"data":{"name":"Ada","email":"ada@example.com","topic":"question","message":"How do workflows work?"}}'
```

Keep the `receiptToken`. It is the only way to read the public status, and it is never shown again.

## Auth, users and API keys

| Method | Path | Auth | Description |
|---|---|---|---|
| POST | `/auth/login` | none | `{email, password}` → `{user}` + cookie |
| POST | `/auth/logout` | session | 204 |
| POST | `/auth/password-reset` | none | `{email}` → 202 always; emails a reset link (valid 1 hour) to `OPENFORMS_BASE_URL/admin/reset-password?token=…` if the user exists |
| POST | `/auth/password-reset/confirm` | none | `{token, password}` → 204; the token is single-use and every session of the user is signed out. 400 `invalid_token` if it is unknown, used or expired |
| GET | `/auth/me` | any | `{"principal": {kind, id, name, email, roles}}` |
| GET / POST | `/users` | admin | List / create `{email, name, password, roles}` |
| PATCH / DELETE | `/users/{id}` | admin | Update `{name?, password?, roles?}` / delete |
| GET / POST | `/api-keys` | admin | List / create `{name, roles}` → `{apiKey, key}` (plaintext `key` shown once) |
| DELETE | `/api-keys/{id}` | admin | Revoke |

## Definitions

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/forms` | any | `{"items": [{slug, title, workflow, public, version, source, updatedAt, submissionCount}]}` |
| GET | `/forms/{slug}` | any | `{"form": {slug, version, source, updatedAt, workflowVersion, definition}}` |
| PUT | `/forms/{slug}` | admin | Body = definition; `?source=ui` optional → `{"item": {kind, slug, version, changed, created}}` |
| GET | `/forms/{slug}/versions` | any | `{"items": [{version, hash, source, createdBy, createdAt}]}` newest first |
| GET | `/forms/{slug}/versions/{n}` | any | One version |
| GET | `/forms/{slug}/submissions.csv` | any | CSV export |
| GET, PUT | `/workflows`, `/workflows/{slug}`, `/workflows/{slug}/versions[/{n}]` | as forms | Workflow equivalents |
| POST | `/definitions/validate` | admin | Body `{forms: [...], workflows: [...]}` → `{"valid": true}` or 422 |
| POST | `/definitions/apply` | admin | Same body; `?dryRun=true`, `?source=cli` → `{"items": [ApplyItem]}`; atomic |
| GET | `/definitions` | admin | Export everything as `{forms, workflows}` |

## Submissions

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/submissions` | any | Filters: `form`, `state`, `assignee` (`<uuid>`, `me`, `none`) |
| POST | `/submissions` | any | Create for any form (public or not): `{"form": slug, "data": {...}}` → `201 {submission, receiptToken}` |
| GET | `/submissions/{id}` | any | `{submission, form, workflow, events, transitions}`; `transitions` shows availability for **you** |
| POST | `/submissions/{id}/transitions` | any | `{transition, fields?, comment?, expectedState?}` → `{submission}` |
| PATCH | `/submissions/{id}/fields` | any | `{"fields": {...}}` (null clears) → `{submission}` |
| POST | `/submissions/{id}/comments` | any | `{"body"}` → `201 {event}` |
| PUT | `/submissions/{id}/assignee` | any | `{"userId": uuid | null}` → `{submission}` |

### Shapes

```jsonc
// Submission
{ "id": "…", "form": "job-application", "formVersion": 2, "state": "screening", "stateLabel": "Screening",
  "terminal": false, "data": { "name": "Ada" }, "fields": { "score": 4 },
  "assignee": { "id": "…", "name": "Riley Reviewer", "email": "reviewer@demo.local" },
  "createdAt": "…", "updatedAt": "…" }

// Event (timeline entry); type: created | transition | fields_updated | assigned | comment | action_succeeded | action_failed
{ "id": 42, "type": "transition", "fromState": "new", "toState": "screening", "transition": "screen",
  "actor": { "type": "user", "id": "…", "name": "Riley Reviewer" }, "payload": { "comment": "Looks strong" },
  "createdAt": "…" }

// AvailableTransition
{ "key": "invite", "label": "Invite to interview", "to": "interview", "toLabel": "Interview",
  "requireFields": ["score"], "allowed": true, "reason": "" }
```

## Jobs

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/jobs?status=failed` | admin | `{"items": [{id, kind, status, attempts, maxAttempts, runAt, lastError, payload, createdAt, updatedAt}]}` |
| POST | `/jobs/{id}/retry` | admin | Re-queue a failed job → 204 |

## Health

`GET /healthz` (outside `/api/v1`) → `{"status":"ok"}` when the database is reachable.
