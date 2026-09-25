# Self-hosting

openforms is one container plus PostgreSQL 16. The server is a Python (FastAPI) application and the web UI ships inside the package.

## Docker Compose (production)

```yaml
services:
  postgres:
    image: postgres:16
    environment:
      POSTGRES_USER: openforms
      POSTGRES_PASSWORD: change-me
      POSTGRES_DB: openforms
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U openforms -d openforms"]
      interval: 5s
      retries: 20
    restart: unless-stopped

  openforms:
    image: ghcr.io/openforms/openforms:latest   # or build: .
    environment:
      OPENFORMS_DATABASE_URL: postgres://openforms:change-me@postgres:5432/openforms?sslmode=disable
      OPENFORMS_BASE_URL: https://forms.example.com
      OPENFORMS_SMTP_HOST: smtp.example.com
      OPENFORMS_SMTP_PORT: "587"
      OPENFORMS_SMTP_USERNAME: forms@example.com
      OPENFORMS_SMTP_PASSWORD: change-me
      OPENFORMS_SMTP_FROM: forms@example.com
      OPENFORMS_WEBHOOK_SECRET: a-long-random-string
    ports:
      - "127.0.0.1:8080:8080"
    depends_on:
      postgres:
        condition: service_healthy
    restart: unless-stopped

volumes:
  pgdata: {}
```

Create the first admin:

```bash
docker compose exec openforms openforms admin create-user \
  --email you@example.com --name "Your Name" --password 'a-strong-password' --roles admin
```

## Configuration

| Variable | Default | Description |
|---|---|---|
| `OPENFORMS_DATABASE_URL` | none (required) | PostgreSQL connection string |
| `OPENFORMS_HTTP_ADDR` | `:8080` | Listen address |
| `OPENFORMS_BASE_URL` | `http://localhost:8080` | Public URL; used in links in emails and webhooks |
| `OPENFORMS_COOKIE_SECURE` | `true` when `BASE_URL` is https | Mark the session cookie `Secure` |
| `OPENFORMS_SMTP_HOST` | none (empty) | SMTP server; when empty, emails (including password reset links) are logged instead of sent |
| `OPENFORMS_SMTP_PORT` | `1025` | SMTP port |
| `OPENFORMS_SMTP_USERNAME` / `OPENFORMS_SMTP_PASSWORD` | none (empty) | SMTP credentials |
| `OPENFORMS_SMTP_FROM` | `openforms@localhost` | Sender address |
| `OPENFORMS_WEBHOOK_SECRET` | none (empty) | HMAC secret for `X-OpenForms-Signature`; when empty, webhooks are unsigned |
| `OPENFORMS_FORWARDED_ALLOW_IPS` | `127.0.0.1` | Reverse proxies whose `X-Forwarded-For`/`X-Forwarded-Proto` are trusted (comma-separated IPs or CIDRs, `*` for any). Set it to your proxy's address so rate limits apply per client, not per proxy |
| `OPENFORMS_WORKER_CONCURRENCY` | `4` | Parallel background jobs per instance |
| `OPENFORMS_DEMO` | `false` | Demo mode: shows demo accounts on the login page and enables the /demo reviewer simulation. **Never enable in production.** |
| `OPENFORMS_SEED_DEMO` | `false` | Seed the demo bundle and demo users on start |

## Reverse proxy and TLS

Put openforms behind a TLS-terminating proxy and set `OPENFORMS_BASE_URL` to the public https URL. With Caddy:

```
forms.example.com {
  reverse_proxy 127.0.0.1:8080
}
```

The proxy must pass the client IP (`X-Forwarded-For`) for per-IP rate limiting of public submissions.

## Upgrades

Pull the new image and restart. `openforms serve` runs pending database migrations on start. To migrate separately, for example in a release job, run `openforms migrate` first.

## Backups

All state lives in PostgreSQL:

```bash
docker compose exec -T postgres pg_dump -U openforms -Fc openforms > openforms-$(date +%F).dump
# restore
docker compose exec -T postgres pg_restore -U openforms -d openforms --clean < openforms-2026-09-23.dump
```

## Running more than one instance

- Background jobs are claimed with `SELECT … FOR UPDATE SKIP LOCKED`, so several instances can share one database safely. A job whose worker dies is picked up again after 5 minutes.
- The public submission rate limit (20 per minute per IP) is kept in memory per instance.
- Webhook delivery is at least once. Receivers should deduplicate on `X-OpenForms-Delivery`.

## Security checklist

- Use a strong database password and keep Postgres off the public network.
- Keep `OPENFORMS_DEMO` and `OPENFORMS_SEED_DEMO` off, since the demo accounts have a published password.
- Set `OPENFORMS_WEBHOOK_SECRET` and verify signatures in your receivers ([workflows](workflows.md#webhook)).
- Give API keys only the roles they need, and revoke unused keys in **Admin → API keys**.
