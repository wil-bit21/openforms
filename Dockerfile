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
RUN mkdir -p internal/webui/dist && touch internal/webui/dist/.gitkeep
RUN cd web && pnpm install --frozen-lockfile && pnpm build
# Refuse to produce an image without the UIs (the server would answer 503).
RUN test -f internal/webui/dist/admin/index.html \
 && test -f internal/webui/dist/hosted/index.html \
 && test -f internal/webui/dist/embed/embed.js

# ---- Stage 2: build the Go binary with the web apps embedded -----------------
FROM golang:1.26-bookworm AS go
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/internal/webui/dist ./internal/webui/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/openforms ./cmd/openforms

# ---- Stage 3: minimal runtime -------------------------------------------------
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=go /out/openforms /openforms
ENV OPENFORMS_HTTP_ADDR=:8080
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/openforms"]
CMD ["serve"]
