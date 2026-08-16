# syntax=docker/dockerfile:1

FROM golang:1.26.4-alpine3.24 AS builder

WORKDIR /src

# Public roots for any custom api_base_url. The MAX chain itself is served by
# the Russian Trusted CA bundle embedded in the binary, so this only matters
# when someone points api_base_url at a different host.
RUN apk add --no-cache ca-certificates

# Dependency manifests are copied first so the module layer stays cached while
# only sources change. The action has no third-party dependencies, so there is
# no go.sum.
COPY go.mod ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

# VERSION reaches the binary only when the image is built by the release
# workflow. GitHub cannot pass build args to an inline `image: Dockerfile`
# action, which is one reason the published action uses a prebuilt image.
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /out/max-action ./cmd/max-action

# The binary is statically linked, so the runtime stage needs nothing but the
# binary and a public root bundle. scratch keeps the image at a few megabytes
# and leaves no shell or package manager in the published artifact.
FROM scratch

LABEL org.opencontainers.image.title="max-action" \
      org.opencontainers.image.description="Send notifications to MAX from GitHub Actions" \
      org.opencontainers.image.source="https://github.com/Fgeeha/max-action" \
      org.opencontainers.image.licenses="MIT"

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /out/max-action /usr/local/bin/max-action

# The runner mounts the checkout here and creates GITHUB_OUTPUT outside the
# container, so the process keeps the default user to stay able to read the
# workspace and append its outputs.
WORKDIR /github/workspace

ENTRYPOINT ["/usr/local/bin/max-action"]
