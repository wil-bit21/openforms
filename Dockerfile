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

# ---- Stage 2: build the wheel (with the web apps inside) ----------------------
FROM python:3.12-slim AS build
RUN pip install --no-cache-dir uv==0.8.*
WORKDIR /src
COPY pyproject.toml uv.lock README.md ./
COPY schemas/ schemas/
COPY examples/ examples/
COPY src/ src/
COPY --from=web /src/src/openforms/server/ui/ src/openforms/server/ui/
RUN uv export --frozen --no-dev --no-emit-project --no-hashes -o /dist/requirements.txt \
 && uv build --wheel --out-dir /dist

# ---- Stage 3: runtime -----------------------------------------------------------
FROM python:3.12-slim
ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    OPENFORMS_HTTP_ADDR=:8080
COPY --from=build /dist/ /tmp/dist/
RUN pip install --no-cache-dir -r /tmp/dist/requirements.txt \
 && pip install --no-cache-dir --no-deps /tmp/dist/*.whl \
 && rm -rf /tmp/dist \
 && useradd --system --uid 65532 --no-create-home --shell /usr/sbin/nologin openforms \
 && python -c "import openforms._paths as p; assert (p.ui_dir() / 'admin' / 'index.html').is_file()"
USER openforms
EXPOSE 8080
ENTRYPOINT ["openforms"]
CMD ["serve"]
