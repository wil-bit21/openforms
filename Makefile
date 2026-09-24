.PHONY: dev-db test lint fmt sync

sync:
	uv sync

dev-db:
	docker compose up -d postgres mailpit

test:
	uv run pytest

lint:
	uv run ruff check
	uv run ruff format --check
	uv run pyright

fmt:
	uv run ruff check --fix
	uv run ruff format

.PHONY: web web-test build up down e2e

web:
	cd web && pnpm install --frozen-lockfile && pnpm build

web-test:
	cd web && pnpm install --frozen-lockfile && pnpm typecheck && pnpm test

# A wheel with the web apps inside (dist/openforms-*.whl).
build: web
	uv build --wheel

up:
	docker compose --profile app up -d --build

down:
	docker compose --profile app down -v

e2e: up
	cd e2e && pnpm install --frozen-lockfile && pnpm exec playwright install chromium && pnpm test
