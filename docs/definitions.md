# Form definitions

A form is a YAML (or JSON) document. The server stores every version and the CLI keeps them in files named `<dir>/forms/<slug>.yaml`, where the file name must equal the slug.

```yaml
slug: job-application
title: Job application
description: Apply to join the team.
workflow: hiring
settings:
  public: true
  submitLabel: Send application
  confirmationMessage: Thanks! We'll be in touch.
fields:
  - key: name
    type: text
    label: Full name
    required: true
    validation: { minLength: 2, maxLength: 100 }
  - key: role
    type: select
    label: Role
    required: true
    options:
      - { value: engineer, label: Engineer }
      - { value: designer, label: Designer }
  - key: portfolio
    type: url
    label: Portfolio URL
    showIf: { field: role, equals: designer }
```

## Top-level keys

| Key | Required | Description |
|---|---|---|
| `slug` | yes | `^[a-z0-9][a-z0-9-]{0,62}$`. Unique per server; used in URLs (`/f/<slug>`). |
| `title` | yes | Shown above the form and in the admin UI. |
| `description` | no | Shown under the title. |
| `workflow` | no | Slug of a [workflow](workflows.md). Without one, submissions stay in the terminal state `submitted`. |
| `settings.public` | yes | `true` lets anyone load and submit the form through the public API, hosted page and embed. `false` hides it (404) from anonymous users; authenticated API clients can still create submissions. |
| `settings.submitLabel` | no | Submit button text. Default `Submit`. |
| `settings.confirmationMessage` | no | Shown after a successful submission. |
| `fields` | yes | Ordered list of fields (see below). |

## Fields

| Key | Required | Description |
|---|---|---|
| `key` | yes | `^[a-zA-Z][a-zA-Z0-9_]{0,63}$`, unique within the form. The key in submission data. |
| `type` | yes | One of the types below. |
| `label` | yes | Visible label. |
| `help` | no | Hint text under the input. |
| `placeholder` | no | Placeholder text. |
| `required` | no | Must be provided when the field is visible. A required `checkbox` must be checked. |
| `options` | select/multiselect | `[{value, label}]`, at least one, values unique. Not allowed on other types. |
| `validation` | no | See below. |
| `showIf` | no | Condition that controls visibility (see below). |

### Field types and value types

| Type | Value in submission data | Notes |
|---|---|---|
| `text` | string | |
| `textarea` | string | Multi-line. |
| `email` | string | Must be a bare address (`ada@example.com`). |
| `url` | string | Absolute `http://` or `https://` URL with a host. |
| `number` | number | JSON number. |
| `date` | string | `YYYY-MM-DD`. |
| `select` | string | Must be one of the option values. |
| `multiselect` | array of strings | Every value must be an option; no duplicates. |
| `checkbox` | boolean | |

### Validation

| Rule | Applies to | Meaning |
|---|---|---|
| `minLength`, `maxLength` | text, textarea, email, url | Length limits in characters. |
| `pattern` | text, textarea, email, url | A regular expression (RE2 syntax) the whole value must match. |
| `min`, `max` | number | Inclusive bounds; `min` ≤ `max`. |

### Conditional fields: `showIf`

```yaml
showIf: { field: role, equals: designer }       # visible when role == "designer"
showIf: { field: role, notEquals: engineer }    # visible when role != "engineer"
showIf: { field: role, in: [designer, product] } # visible when role is one of these
```

Rules:

- `field` must refer to a field declared **earlier** in the list.
- Use exactly one of `equals`, `notEquals`, `in`.
- When the controlling field is a `multiselect`, `equals: x` means "contains x".
- If the controlling field is itself hidden, the dependent field is hidden too.
- Hidden fields are never required, and their values are dropped from the stored submission.

## What gets stored

When a submission arrives the server:

1. drops keys that are not fields of the form, and values of hidden fields;
2. treats `""`, `null` and `[]` as "not provided";
3. checks types, options, validation rules and required fields.

If anything fails, the API answers `422 validation_failed` with one problem per field, using paths like `data.email`. The TypeScript SDK runs the same rules in the browser for instant feedback, but the server's answer is authoritative.

## Versions

Every change to a definition creates a new immutable version. Applying identical content, compared by a canonical-JSON SHA-256 hash, creates nothing. Each submission remembers the form version and workflow version it was created with, so editing a form never changes how past submissions are displayed or processed.

The admin UI shows each version's **source** (`cli`, `ui`, `api` or `seed`). If a form was last pushed from the CLI, the editor warns that UI edits will be overwritten by the next `openforms push` unless you `openforms pull` first.

## JSON Schema

`schemas/form.schema.json` and `schemas/workflow.schema.json` describe the structure and work with editors that support YAML schemas. For example, with the VS Code YAML extension:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/openforms/openforms/main/schemas/form.schema.json
```
