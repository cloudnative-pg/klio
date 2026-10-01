# syntax=docker/dockerfile:1@sha256:4edf897a3ffa55b89f906fc8cc78afdb3f1834cc9c7083565e611a8a7d5fe99e

ARG BASE_IMAGE=debian:13.7-slim@sha256:a99cfc517144bc59b1978475ec53b46ecabec7e43635402ee5b77cc54cd1b20a

# Build Kopia on the host platform for the target platform
FROM --platform=${BUILDPLATFORM} golang:1.27.1@sha256:e0174e51e81218523251d85d248a90d24c3d5e81543b4f07a5d66229397db190 AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace

# Clone and build Kopia with cache mounts and cross-compilation
RUN --mount=type=cache,target=/go/pkg/mod,id=kopia-mod-${TARGETARCH} \
    --mount=type=cache,target=/root/.cache/go-build,id=kopia-build-${TARGETARCH} \
    git clone --depth=1 --branch=klio https://github.com/cloudnative-pg/kopia && \
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
