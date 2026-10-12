# syntax=docker/dockerfile:1.7

ARG GO_VERSION=1.26
ARG DEBIAN_VERSION=bookworm

FROM golang:${GO_VERSION}-${DEBIAN_VERSION} AS base
WORKDIR /src
ENV GOFLAGS=-mod=readonly \
    GOTOOLCHAIN=local
RUN printf '\n==> [base] %s\n' "$(go version)"

FROM base AS deps
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    set -eu; \
    printf '\n==> [deps] downloading modules\n'; \
    go mod download; \
    go mod verify; \
    printf '==> [deps] %s modules ready\n' "$(go list -m all | wc -l)"

FROM deps AS test
ARG RACE=on
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    RACE=${RACE} bash scripts/test.sh

FROM test AS build
ARG TARGETOS=linux
ARG TARGETARCH
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    set -eu; \
    printf '\n==> [build] soundmate for %s/%s\n' "$TARGETOS" "${TARGETARCH:-$(go env GOARCH)}"; \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=${TARGETARCH:-$(go env GOARCH)} go build \
      -trimpath \
      -ldflags "-s -w" \
      -o /out/soundmate ./cmd/soundmate; \
    printf '==> [build] binary size: %s\n' "$(du -h /out/soundmate | cut -f1)"

FROM gcr.io/distroless/static-debian12:nonroot AS runtime
LABEL org.opencontainers.image.title="soundmate" \
      org.opencontainers.image.description="Discord music bot with a button-driven player" \
      org.opencontainers.image.source="https://github.com/cyblster/soundmate" \
      org.opencontainers.image.licenses="MIT"
COPY --from=build /out/soundmate /usr/local/bin/soundmate
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/soundmate"]
