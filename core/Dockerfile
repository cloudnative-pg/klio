# syntax=docker/dockerfile:1@sha256:2780b5c3bab67f1f76c781860de469442999ed1a0d7992a5efdf2cffc0e3d769

ARG BASE_IMAGE=registry.access.redhat.com/ubi10/ubi-micro:10.2-1777510731@sha256:00a64774d3eb664f673834331d81eb160b47b28bc3c87893ac77a1e8c197d234

# Build Kopia on the host platform for the target platform
FROM --platform=${BUILDPLATFORM} golang:1.26.3@sha256:cc9a5d7a008cfe2cbc7ffc752b0d6636ad30fc16e4a648d2e4aac00fd8b25ca3 AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace

# Clone and build Kopia with cache mounts and cross-compilation
RUN --mount=type=cache,target=/go/pkg/mod,id=kopia-mod-${TARGETARCH} \
    --mount=type=cache,target=/root/.cache/go-build,id=kopia-build-${TARGETARCH} \
    git clone --depth=1 --branch=klio-20260513 https://github.com/leonardoce/kopia && \
    cd kopia && \
    CGO_ENABLED=0 GOOS="${TARGETOS}" GOARCH="${TARGETARCH}" \
    go build -o kopia main.go

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
