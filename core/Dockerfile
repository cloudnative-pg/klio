# syntax=docker/dockerfile:1@sha256:572cebbc2b15e6f84c077742952674889cb63cd5e9b80abdffcbe587b6f19183

ARG BASE_IMAGE=debian:13.8-slim@sha256:8aa5a7bf5a1955bddbe7d74fcdd0b91ab30e74362a8b73af672b0481b35b44a0

# renovate: datasource=git-refs depName=kopia lookupName=https://github.com/cloudnative-pg/kopia currentValue=klio
ARG KOPIA_SHA=4181abf25403d7e92c2dfacbee98e1489b172073

# Build Kopia on the host platform for the target platform
FROM --platform=${BUILDPLATFORM} golang:1.27.4@sha256:5f1b98fa02fe396ebce2a0227a0e08309e11792915b9dd2e1f71053d7e7e6666 AS builder

ARG TARGETOS
ARG TARGETARCH
ARG KOPIA_SHA

WORKDIR /workspace

# Fetch Kopia at the pinned commit and build it with cache mounts and
# cross-compilation. Building the package lets Go stamp the commit.
RUN --mount=type=cache,target=/go/pkg/mod,id=kopia-mod-${TARGETARCH} \
    --mount=type=cache,target=/root/.cache/go-build,id=kopia-build-${TARGETARCH} \
    git init -q kopia && \
    cd kopia && \
    git fetch -q --depth=1 https://github.com/cloudnative-pg/kopia "${KOPIA_SHA}" && \
    git checkout -q FETCH_HEAD && \
    CGO_ENABLED=0 GOOS="${TARGETOS}" GOARCH="${TARGETARCH}" \
    go build -o kopia .

FROM ${BASE_IMAGE} AS base
ARG TARGETARCH
ARG TARGETOS
# Copy CA certificates from builder image to support TLS connections
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /workspace/kopia/kopia /usr/bin/kopia
COPY dist/klio_${TARGETOS}_${TARGETARCH}*/klio /usr/bin/klio
COPY licenses /licenses
ENTRYPOINT ["/usr/bin/klio"]
USER 26
