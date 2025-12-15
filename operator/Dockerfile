# syntax=docker/dockerfile:1

ARG BASE_IMAGE=registry.access.redhat.com/ubi10/ubi-micro:10.1-1765178423@sha256:d5ba6a075880deb061ac2c7d89c7c409e7948b3abd60b591578621c1b6307b62

# Build stage: build on the host platform for the target platform
FROM --platform=${BUILDPLATFORM} golang:1.25@sha256:36b4f45d2874905b9e8573b783292629bcb346d0a70d8d7150b6df545234818f AS builder
WORKDIR /workspace

ARG TARGETPLATFORM
ARG BUILDPLATFORM
ARG TARGETOS
ARG TARGETARCH

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum

# Download dependencies with cache mount for go mod cache
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Copy the go source
COPY cmd/main.go cmd/main.go
COPY api/ api/
COPY internal/ internal/
COPY pkg/ pkg/

# Build the binary for the target platform with cache mounts
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -o manager cmd/main.go

FROM ${BASE_IMAGE}
WORKDIR /
COPY --from=builder /workspace/manager .
COPY licenses /licenses
USER 65532:65532

ENTRYPOINT ["/manager"]
