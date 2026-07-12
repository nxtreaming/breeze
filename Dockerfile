# syntax=docker/dockerfile:1

# ─── Build stage ──────────────────────────────────────────────────────────────
# Breeze is a library; this image builds and runs the example server in ./cmd
# as a proof of concept. Point BREEZE_TARGET at any other main package
# (e.g. ./cmd/dashboard-example) to containerize a different app.
ARG GO_VERSION=1.24
FROM golang:${GO_VERSION}-alpine AS build

ARG BREEZE_TARGET=./cmd

WORKDIR /src

# Cache module downloads separately from source changes.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

# gnet is pure Go, so a fully static binary works.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/breeze-server ${BREEZE_TARGET}

# ─── Runtime stage ────────────────────────────────────────────────────────────
FROM alpine:3.22

RUN apk add --no-cache ca-certificates \
    && addgroup -S breeze && adduser -S breeze -G breeze

WORKDIR /app

COPY --from=build /out/breeze-server /usr/local/bin/breeze-server

# The example serves ./files via router.ServeStatic("/files", "./files/").
RUN mkdir -p /app/files && chown -R breeze:breeze /app

USER breeze

EXPOSE 3000

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -qO- http://127.0.0.1:3000/ >/dev/null || exit 1

ENTRYPOINT ["breeze-server"]
