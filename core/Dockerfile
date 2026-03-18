# syntax=docker/dockerfile:1@sha256:4a43a54dd1fedceb30ba47e76cfcf2b47304f4161c0caeac2db1c61804ea3c91

ARG BASE_IMAGE=registry.access.redhat.com/ubi10/ubi-micro:10.1-1769518576@sha256:551f8ee81be3dbabd45a9c197f3724b9724c1edb05d68d10bfe85a5c9e46a458

# Build Kopia on the host platform for the target platform
FROM --platform=${BUILDPLATFORM} golang:1.26.1@sha256:318ba178e04ea7655d4e4b1f3f0e81da0da5ff28a2c48681ff0418fb75f5e189 AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace

# Clone and build Kopia with cache mounts and cross-compilation
RUN --mount=type=cache,target=/go/pkg/mod,id=kopia-mod-${TARGETARCH} \
    --mount=type=cache,target=/root/.cache/go-build,id=kopia-build-${TARGETARCH} \
    git clone --depth=1 --branch=klio-20260304 https://github.com/leonardoce/kopia && \
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
