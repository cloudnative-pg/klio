# syntax=docker/dockerfile:1@sha256:b6afd42430b15f2d2a4c5a02b919e98a525b785b1aaff16747d2f623364e39b6

ARG BASE_IMAGE=registry.access.redhat.com/ubi10/ubi-micro:10.1-1766049088@sha256:2946fa1b951addbcd548ef59193dc0af9b3e9fedb0287b4ddb6e697b06581622

# Build stage: build on the host platform for the target platform
FROM --platform=${BUILDPLATFORM} golang:1.25.5@sha256:31c1e53dfc1cc2d269deec9c83f58729fa3c53dc9a576f6426109d1e319e9e9a AS builder
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
