# syntax=docker/dockerfile:1

# ---- Builder stage: full Go toolchain, discarded after the build ----
FROM golang:1.23-alpine AS builder

# Injected at build time: docker build --build-arg VERSION=0.1.0 .
ARG VERSION=dev

WORKDIR /src

# Dependencies first: this layer is cached until go.mod or go.sum changes.
COPY go.mod go.sum ./
RUN go mod download

# Then the source, so editing code does not re-download the module cache.
COPY cmd/ ./cmd/
COPY internal/ ./internal/

# CGO_ENABLED=0 is what makes the binary static: no libc, no dynamic loader.
# -s -w strip the symbol table and DWARF data: 13.7 MB -> 9.4 MB, which is what
# keeps the final image under the 20 MB target.
# -X writes the build version into main.Version.
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.Version=${VERSION}" \
    -o /app/mqtt2bdd ./cmd/mqtt2bdd

# ---- Runtime stage: the binary and a CA bundle, nothing else ----
FROM alpine:3.23 AS runtime

# ARG does not cross a FROM boundary; without this line the version label below
# expands to an empty string.
ARG VERSION=dev

# TLS trust store, for a future TLS-enabled broker or database. --no-cache keeps
# the apk index out of the layer.
RUN apk add --no-cache ca-certificates

# Arrives root-owned and 0755, which is exactly what USER nobody needs.
COPY --from=builder /app/mqtt2bdd /app/mqtt2bdd

LABEL org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.description="MQTT to PostgreSQL bridge for home-automation telemetry" \
      org.opencontainers.image.authors="Kevin Hagner <kevin.hagner@spyzone.fr>"

# After apk and COPY, both of which need root. nobody is UID/GID 65534 on Alpine.
USER nobody

# Exec form, so the binary is PID 1 and receives SIGTERM directly — which is
# what the graceful shutdown in main.go depends on. Shell form would interpose /bin/sh.
ENTRYPOINT ["/app/mqtt2bdd"]
