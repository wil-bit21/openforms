# syntax=docker/dockerfile:1.7

# ---- Stage 1: build the web apps --------------------------------------------
FROM node:22-bookworm-slim AS web
WORKDIR /src
RUN corepack enable && corepack prepare pnpm@9 --activate
# The SDK's generated types are committed; schemas/ is only needed by tests.
# The demo app imports YAML from examples/ via Vite ?raw imports.
COPY schemas/ schemas/
COPY examples/ examples/
COPY web/ web/
RUN mkdir -p src/openforms/server/ui
RUN cd web && pnpm install --frozen-lockfile && pnpm build
# Refuse to produce an image without the UIs (the server would answer 503).
RUN test -f src/openforms/server/ui/admin/index.html \
 && test -f src/openforms/server/ui/demo/index.html \
 && test -f src/openforms/server/ui/hosted/index.html \
 && test -f src/openforms/server/ui/embed/embed.js

# ---- Stage 2: install the package and its locked dependencies with uv --------
FROM python:3.12-slim AS build
COPY --from=ghcr.io/astral-sh/uv:0.8 /uv /uvx /bin/
ENV UV_COMPILE_BYTECODE=1 \
    UV_LINK_MODE=copy \
    UV_PYTHON_DOWNLOADS=never \
    UV_PROJECT_ENVIRONMENT=/opt/venv
WORKDIR /src
COPY pyproject.toml uv.lock README.md LICENSE ./
# Dependencies first so this layer is cached until uv.lock changes.
RUN uv sync --frozen --no-dev --no-install-project
COPY schemas/ schemas/
COPY examples/ examples/
COPY src/ src/
COPY --from=web /src/src/openforms/server/ui/ src/openforms/server/ui/
# The project itself (with the web UI inside), installed like a wheel rather than editable.
RUN uv sync --frozen --no-dev --no-editable

# ---- Stage 3: runtime ------------------------------------------------------------
FROM python:3.12-slim
ENV PATH=/opt/venv/bin:$PATH \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    OPENFORMS_HTTP_ADDR=:8080
COPY --from=build /opt/venv /opt/venv
RUN useradd --uid 65532 --no-create-home --shell /usr/sbin/nologin openforms \
 && python -c "import openforms._paths as p; assert (p.ui_dir() / 'admin' / 'index.html').is_file()"
USER openforms
EXPOSE 8080
ENTRYPOINT ["openforms"]
CMD ["serve"]
