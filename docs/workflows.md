# Workflows

A workflow is a state machine attached to a form. It decides what states a submission can be in, who can move it between them, what data reviewers must provide, and what happens automatically.

```yaml
slug: hiring
title: Hiring pipeline
initial: new
states:
  - { key: new, label: New, color: gray }
  - { key: screening, label: Screening, color: blue }
  - { key: hired, label: Hired, color: green, terminal: true }
  - { key: rejected, label: Rejected, color: red, terminal: true }
fields:
  - { key: score, type: number, label: Score }
  - { key: rejectionReason, type: textarea, label: Rejection reason }
onSubmit:
  - { type: assign, role: reviewer }
transitions:
  - key: screen
    label: Start screening
    from: [new]
    to: screening
    guard: { roles: [reviewer] }
  - key: hire
    label: Hire
    from: [screening]
    to: hired
    guard: { roles: [hiring-manager], requireFields: [score] }
    actions:
      - { type: webhook, url: "https://hr.example.com/hooks/hired" }
  - key: reject
    label: Reject
    from: [new, screening]
    to: rejected
    guard: { roles: [reviewer, hiring-manager], requireFields: [rejectionReason] }
    actions:
      - type: email
        to: "{{submission.data.email}}"
        subject: Your application
        body: "{{submission.fields.rejectionReason}}"
```

Attach it to a form with `workflow: hiring` in the form definition.

## States

| Key | Required | Description |
|---|---|---|
| `key` | yes | `^[a-zA-Z][a-zA-Z0-9_]{0,63}$`, unique. |
| `label` | yes | Shown to reviewers and on the respondent's status page. |
| `color` | no | `gray`, `blue`, `green`, `yellow`, `red` or `purple`, used for badges and the diagram. |
| `terminal` | no | A final state. Transitions may not leave it. |

`initial` names the state every new submission starts in.

## Workflow fields

`fields` declares data that reviewers add to a submission over time, separate from the respondent's answers. Allowed types are `text`, `textarea`, `number`, `select` (with `options`), `checkbox` and `date`. Reviewers can edit them in the admin UI (`PATCH /submissions/{id}/fields`) or provide them while performing a transition. Setting a field to `null` clears it.

## Transitions

| Key | Required | Description |
|---|---|---|
| `key` | yes | Unique; used in API calls. |
| `label` | yes | Button text. |
| `from` | yes | Non-empty list of states the transition is available from. |
| `to` | yes | Target state. |
| `guard.roles` | no | Who may perform it. Empty means any authenticated user. Users and API keys with the `admin` role always pass. |
| `guard.requireFields` | no | Workflow fields that must have a value **after** the transition's own `fields` are merged in. |
| `actions` | no | Side effects run after the transition commits. |

### How a transition runs

`POST /api/v1/submissions/{id}/transitions` with `{"transition": "reject", "fields": {"rejectionReason": "…"}, "comment": "…", "expectedState": "screening"}`:

1. The submission row is locked, so two reviewers can't move it at once.
2. Unknown transition → `422 unknown_transition`.
3. `expectedState` given and different from the current state → `409 state_conflict`. Use it to avoid acting on stale screens.
4. Current state not in `from` → `409 invalid_state`.
5. Caller lacks every role in `guard.roles` → `403 forbidden`.
6. `fields` are type-checked and merged; any missing `requireFields` → `422 validation_failed` with paths `fields.<key>`.
7. The state and fields are saved, a `transition` event with the comment is added to the timeline, and every action is queued, all in one database transaction.

## Actions

Actions run in a background worker. They are queued in the same transaction as the state change, so they only run when the change commits, and they are retried when they fail.

### `email`

```yaml
- type: email
  to: "{{submission.data.email}}"
  subject: "Update on your application"
  body: |
    Hi {{submission.data.name}},
    Your application is now {{submission.stateLabel}}.
```

Plain-text email over SMTP (see [self-hosting](self-hosting.md)). Without SMTP settings, emails are written to the server log.

### `webhook`

```yaml
- { type: webhook, url: "https://example.com/hooks/openforms" }
```

The server sends `POST <url>` with a JSON body:

```json
{
  "event": "submission.transitioned",
  "submission": { "id": "…", "form": "hiring", "formVersion": 3, "state": "rejected", "stateLabel": "Rejected",
                  "terminal": true, "data": { … }, "fields": { … }, "assignee": null,
                  "createdAt": "…", "updatedAt": "…" },
  "transition": { "key": "reject", "label": "Reject", "from": "screening", "to": "rejected" },
  "form": { "slug": "job-application", "title": "Job application" }
}
```

`event` is `submission.created` for `onSubmit` actions, and `transition` is then `null`.

Headers:

| Header | Value |
|---|---|
| `X-OpenForms-Event` | `submission.created` or `submission.transitioned` |
| `X-OpenForms-Delivery` | Unique delivery id. Use it to deduplicate, because delivery is at least once. |
| `X-OpenForms-Signature` | `sha256=<hex HMAC-SHA256 of the raw body>` when `OPENFORMS_WEBHOOK_SECRET` is set |

Verify signatures against the **raw** request body:

```ts
// Node.js
import { createHmac, timingSafeEqual } from "node:crypto";

export function verify(rawBody: Buffer, header: string | undefined, secret: string): boolean {
  if (!header?.startsWith("sha256=")) return false;
  const expected = Buffer.from("sha256=" + createHmac("sha256", secret).update(rawBody).digest("hex"));
  const given = Buffer.from(header);
  return expected.length === given.length && timingSafeEqual(expected, given);
}
```

```python
# Python
import hashlib, hmac


def verify(raw_body: bytes, header: str | None, secret: str) -> bool:
    expected = "sha256=" + hmac.new(secret.encode(), raw_body, hashlib.sha256).hexdigest()
    return header is not None and hmac.compare_digest(expected, header)
```

```go
// Go
func Verify(body []byte, header, secret string) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(header))
}
```

Responses: any `2xx` is success. `408`, `429`, `5xx` and network errors are retried. Other `4xx` responses fail immediately. Requests time out after 10 seconds.

### `assign`

```yaml
- { type: assign, role: reviewer }          # least-loaded user with the role
- { type: assign, user: ada@example.com }   # a specific user
```

With `role`, openforms picks the user with that role who has the fewest assigned non-terminal submissions (ties go to the earliest-created user). If nobody has the role, it picks from admins. An unknown `user` email fails the action without retries.

### `onSubmit`

`onSubmit` actions run once when a submission is created. They are commonly used to auto-assign and send an acknowledgement email.

### Retries and failures

A failing action is retried with exponential backoff (5 s, 10 s, 20 s, … capped at 1 hour), up to 8 attempts. Every successful action adds an `action_succeeded` event to the submission's timeline. When an action finally fails, an `action_failed` event with the error is added and the job appears under **Jobs** in the admin UI, where an admin can retry it.

## Template variables

`email` actions render `{{…}}` placeholders in `to`, `subject` and `body`. Unknown variables render as an empty string.

| Variable | Value |
|---|---|
| `submission.id` | Submission UUID |
| `submission.state` / `submission.stateLabel` | Current state key / label (after the transition) |
| `submission.data.<key>` | The respondent's answer |
| `submission.fields.<key>` | A workflow field value |
| `submission.url` | Admin link: `<BASE_URL>/admin/submissions/<id>` |
| `form.slug`, `form.title` | The form |
| `transition.key`, `transition.label` | The transition being performed (empty for `onSubmit`) |
| `baseUrl` | `OPENFORMS_BASE_URL` |

## Versions and in-flight submissions

A submission keeps using the workflow version it was created with. If you rename or remove a state, existing submissions still follow the old rules, and new submissions use the new version. Applying a new workflow version automatically creates a new version of every form that references it.
