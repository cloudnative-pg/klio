# syntax=docker/dockerfile:1@sha256:2780b5c3bab67f1f76c781860de469442999ed1a0d7992a5efdf2cffc0e3d769

ARG BASE_IMAGE=registry.access.redhat.com/ubi10/ubi-micro:10.1-1778532954@sha256:38786abef22069f3dbbf286f2ccea07e31bce9b5cac53fb4f170e0cc54e1030d

# Build stage: build on the host platform for the target platform
FROM --platform=${BUILDPLATFORM} golang:1.26.3@sha256:633d23bf362cb40dd72b4f277288a8929697d77537f9c801b81aeced19b5bdf3 AS builder
WORKDIR /workspace

ARG TARGETOS
ARG TARGETARCH

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum

# Download dependencies with architecture-specific cache
RUN --mount=type=cache,target=/go/pkg/mod,id=operator-mod-${TARGETARCH} \
    go mod download

# Copy the go source
COPY cmd/main.go cmd/main.go
COPY api/ api/
COPY internal/ internal/
COPY pkg/ pkg/

# Build the binary for the target platform with architecture-specific caches
RUN --mount=type=cache,target=/go/pkg/mod,id=operator-mod-${TARGETARCH} \
    --mount=type=cache,target=/root/.cache/go-build,id=operator-build-${TARGETARCH} \
    CGO_ENABLED=0 GOOS="${TARGETOS}" GOARCH="${TARGETARCH}" \
    go build -o manager cmd/main.go

FROM ${BASE_IMAGE}
WORKDIR /
COPY --from=builder /workspace/manager .
COPY licenses /licenses
USER 65532:65532

ENTRYPOINT ["/manager"]
