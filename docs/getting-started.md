# Getting started

This guide takes you from nothing to a working form with a review workflow in about ten minutes.

## 1. Run openforms

You need Docker. From a clone of the repository:

```bash
docker compose --profile app up -d --build
curl http://localhost:8080/healthz   # {"status":"ok"}
```

The compose stack starts PostgreSQL, Mailpit (a local email catcher at http://localhost:8025) and openforms in demo mode. Demo mode seeds two example forms and three accounts. Their password is `demo1234`:

| Email | Role |
|---|---|
| `admin@demo.local` | admin |
| `reviewer@demo.local` | reviewer |
| `manager@demo.local` | hiring-manager |

Open http://localhost:8080/demo for a guided tour, or sign in at http://localhost:8080/admin.

> Running without Docker? Build with `make build`, start Postgres, then run
> `OPENFORMS_DATABASE_URL=postgres://… ./bin/openforms serve`. See [self-hosting](self-hosting.md).

## 2. Create an API key

The CLI talks to the server with an API key. Create one with the `admin` role:

```bash
docker compose exec openforms /openforms admin create-api-key --name my-laptop --roles admin
# ofk_2V9s…   ← shown once; copy it
export OPENFORMS_URL=http://localhost:8080
export OPENFORMS_API_KEY=ofk_2V9s…
```

You can also create keys in the admin UI under **API keys**.

## 3. Scaffold definitions

Install the CLI (`make build` produces `bin/openforms`, or use the Docker image) and, in your own project:

```bash
openforms init
```

This writes:

```
openforms.yaml                           # server URL and definitions directory
openforms/forms/contact.yaml             # a contact form
openforms/workflows/contact-triage.yaml  # its review workflow
```

## 4. Validate and push

```bash
openforms validate      # offline; prints file:path: message for every problem
openforms push --dry-run
openforms push
```

`push` prints one line per definition, with a status of `created`, `updated` or `unchanged`. Pushing the same files again changes nothing: definitions are versioned by content hash.

## 5. Collect a submission

Open http://localhost:8080/f/contact, fill it in and submit. The confirmation page links to a private status page (`/s/<id>?token=…`) that the respondent can bookmark.

Other ways to collect submissions:

- **Embed** the form on any site:
  ```html
  <div data-openforms="contact"></div>
  <script src="http://localhost:8080/embed.js" async></script>
  ```
- **Render it yourself** with React:
  ```tsx
  import { OpenFormsClient } from "@openforms/sdk";
  import { OpenForm } from "@openforms/react";
  import "@openforms/react/styles.css";

  const client = new OpenFormsClient({ baseUrl: "https://forms.example.com" });
  export const Contact = () => <OpenForm client={client} slug="contact" />;
  ```
- **Call the API** directly: `POST /api/v1/public/forms/contact/submissions` with `{"data": {...}}`.

## 6. Review it

Sign in at http://localhost:8080/admin. The inbox lists every submission. Open one to see its answers and timeline, and the transitions your role allows. A transition that needs fields (such as a reply) asks for them before it runs. Its actions, such as sending an email, run in the background. Check Mailpit to see the email arrive.

## 7. Keep definitions in git

Commit the `openforms/` directory. In CI, fail the build when someone edited a form in the UI without pulling:

```bash
openforms validate
openforms pull --check   # exits 1 if the server differs from your files
```

## Next steps

- [Form definitions](definitions.md): every field type, validation rule and condition.
- [Workflows](workflows.md): states, guards, actions, templates and webhooks.
- [HTTP API](api.md) and [CLI](cli.md) reference.
- [Self-hosting](self-hosting.md) for production.
