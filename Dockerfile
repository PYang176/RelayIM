# syntax=docker/dockerfile:1

FROM golang:1.26.11-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY cmd ./cmd
COPY internal ./internal

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/relayim \
    ./cmd/server

FROM golang:1.26.11-alpine AS goose-builder

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOBIN=/out \
    go install github.com/pressly/goose/v3/cmd/goose@v3.24.3

FROM alpine:3.24 AS runtime-base

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S -g 10001 relayim \
    && adduser -S -D -H -u 10001 -G relayim relayim

FROM runtime-base AS runtime

WORKDIR /app

COPY --from=builder --chown=10001:10001 /out/relayim ./relayim
COPY --from=builder --chown=10001:10001 \
    /src/internal/config/app.example.yml \
    ./internal/config/app.yml

USER 10001:10001

EXPOSE 8081

HEALTHCHECK --interval=15s --timeout=3s --start-period=20s --retries=3 \
    CMD wget -q -O - http://127.0.0.1:8081/health/ready >/dev/null || exit 1

ENTRYPOINT ["/app/relayim"]

FROM runtime-base AS migration

COPY --from=goose-builder /out/goose /usr/local/bin/goose
COPY --chown=10001:10001 migrations /migrations

USER 10001:10001

WORKDIR /migrations
