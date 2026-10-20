FROM node:24-bookworm-slim AS web
WORKDIR /src
RUN corepack enable && corepack prepare pnpm@11.28.5 --activate
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile
COPY tsconfig.json vite.config.ts ./
COPY src ./src
RUN pnpm run build:web

FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/internal/webui/assets/dist ./internal/webui/assets/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/uniproxy ./cmd/uniproxy
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/core-install ./cmd/core-install

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/* && useradd --system --uid 10001 --create-home uniproxy
WORKDIR /app
COPY --from=build /out/uniproxy /app/uniproxy
COPY --from=build /out/core-install /app/core-install
RUN mkdir -p /app/bin /data && /app/core-install -output /app/bin/mihomo && chown -R uniproxy:uniproxy /data /app/bin
USER uniproxy
# /data is the container's mounted volume; the host default (/var/lib/uniproxy)
# is replaced here on purpose because the volume must be a single mount point.
ENV UNIPROXY_HOST=0.0.0.0 UNIPROXY_PORT=9090 UNIPROXY_DATA_DIR=/data UNIPROXY_CORE_PATH=/app/bin/mihomo
EXPOSE 9090/tcp
VOLUME ["/data"]
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s CMD ["/app/uniproxy", "-healthcheck"]
ENTRYPOINT ["/app/uniproxy"]
