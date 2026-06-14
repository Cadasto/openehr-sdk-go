# syntax=docker/dockerfile:1.7
#
# Developer toolchain image for openehr-sdk-go.
#
# This is a library module — there is no service binary to ship. The image
# exists so the Makefile can shell Go toolchain calls (gofmt, go vet, go
# test, go build, go mod) through it when a host Go 1.25.x install is not
# available.
#
# golangci-lint is installed from the same pinned image the Makefile uses
# (LINT_IMAGE), so the dev container carries the full toolchain: go, gofmt,
# and golangci-lint v2 with its bundled formatters (gofumpt + goimports).
# One pinned version keeps `make fmt` / `make lint` reproducible whether they
# run on the host or in this image.

# Pin the PATCH version for reproducible builds (matches go.mod's `go`
# line). Bump explicitly when a new stable patch ships — same policy as
# the Makefile's LINT_IMAGE pin.
ARG GO_VERSION=1.25.0
ARG ALPINE_VERSION=3.20
ARG GOLANGCI_IMAGE=golangci/golangci-lint:v2.11.4-alpine

FROM ${GOLANGCI_IMAGE} AS golangci

FROM golang:${GO_VERSION}-alpine AS dev

# Match the WSL/Linux host user so files created in /workspace are owned
# by the developer on the host. Override at build time with
# --build-arg USER_UID=$(id -u) USER_GID=$(id -g) if your host differs.
ARG USER_UID=1000
ARG USER_GID=1000

RUN apk add --no-cache git make ca-certificates tzdata \
    && addgroup -g ${USER_GID} dev \
    && adduser -D -u ${USER_UID} -G dev -s /bin/sh dev \
    && mkdir -p /go/pkg/mod /go/bin /home/dev/.cache/go-build /workspace \
    && chown -R dev:dev /go /home/dev /workspace

# golangci-lint v2 (pinned, matches the Makefile LINT_IMAGE) — provides
# `make lint` and the `make fmt` formatters (gofumpt + goimports) inside the
# dev container, so the host-missing fallback has the full toolchain.
COPY --from=golangci /usr/bin/golangci-lint /usr/local/bin/golangci-lint

ENV GOPATH=/go \
    GOCACHE=/home/dev/.cache/go-build \
    GOMODCACHE=/go/pkg/mod \
    CGO_ENABLED=0 \
    PATH=/go/bin:/usr/local/go/bin:$PATH

USER dev
WORKDIR /workspace

# Long-running default so `docker compose --profile dev up -d go` keeps
# the container around for interactive `docker compose exec go sh`.
# Overridden by every `docker compose run --rm go <cmd>` invocation.
CMD ["sleep", "infinity"]
