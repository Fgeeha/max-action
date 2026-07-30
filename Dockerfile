# syntax=docker/dockerfile:1

FROM golang:1.24.4-alpine3.22 AS builder

WORKDIR /src

# Dependency manifests are copied first so the module layer stays cached while
# only sources change. The action has no third-party dependencies, so there is
# no go.sum.
COPY go.mod ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /out/max-action ./cmd/max-action

FROM alpine:3.24

LABEL org.opencontainers.image.title="max-action" \
      org.opencontainers.image.description="Send notifications to MAX from GitHub Actions" \
      org.opencontainers.image.source="https://github.com/Fgeeha/max-action" \
      org.opencontainers.image.licenses="MIT"

# Public roots for any custom api_base_url. The MAX chain itself is served by
# the Russian Trusted CA bundle embedded in the binary.
RUN apk add --no-cache ca-certificates

COPY --from=builder /out/max-action /usr/local/bin/max-action

# The runner mounts the checkout here and creates GITHUB_OUTPUT outside the
# container, so the process keeps the default user to stay able to read the
# workspace and append its outputs.
WORKDIR /github/workspace

ENTRYPOINT ["/usr/local/bin/max-action"]
