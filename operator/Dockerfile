# syntax=docker/dockerfile:1@sha256:ecfaec9ed6d810b56388c508f4121597bfbba70d41a6dfeee4d8cad5f295fc32

ARG BASE_IMAGE=registry.access.redhat.com/ubi10/ubi-micro:10.2-1786324819@sha256:cabedb588644e9da2c95ebb173a67b78d58aaedcb0eaa42a86f880bcef8a0b2f

# Build stage: build on the host platform for the target platform
FROM --platform=${BUILDPLATFORM} golang:1.26.5@sha256:7caba5286b4c3613a337b709c573047d8ae62ee76106647313b61e72b99f20af AS builder
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
