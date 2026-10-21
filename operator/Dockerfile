# syntax=docker/dockerfile:1@sha256:4edf897a3ffa55b89f906fc8cc78afdb3f1834cc9c7083565e611a8a7d5fe99e

ARG BASE_IMAGE=gcr.io/distroless/static-debian13:nonroot@sha256:d62067879de890a1def9da0e7766b7d20810a5e5555cef8506b752437d7a3e88

# Build stage: build on the host platform for the target platform
FROM --platform=${BUILDPLATFORM} golang:1.27.3@sha256:f20d83a83e18dbd2a4714b8ffd4b30b287b6243962dd75608e86abcc63ba4efc AS builder
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
