# Stage 1: Build Shadcn UI web frontend
FROM node:22-alpine AS web-builder

WORKDIR /src/web
RUN corepack enable && corepack prepare pnpm@latest --activate

COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile

COPY web/ ./
RUN pnpm build

# Stage 2: Build static pure-Go binary (CGO_ENABLED=0)
FROM golang:1.24-alpine AS go-builder

ARG APP_VERSION=3.0.0
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY *.go LICENSE ./
COPY internal ./internal
COPY assets ./assets
COPY --from=web-builder /src/web/dist ./web/dist

RUN CGO_ENABLED=0 go build \
    -ldflags="-s -w -X mmonitor/internal/config.AppVersion=${APP_VERSION}" \
    -o /out/mmonitor .

# Stage 3: Minimal runtime image
FROM alpine:3.21

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app
COPY --from=go-builder /out/mmonitor /usr/local/bin/mmonitor

VOLUME ["/data"]
EXPOSE 8080

ENTRYPOINT ["mmonitor"]
