# syntax=docker/dockerfile:1

ARG BASE_IMAGE=registry.access.redhat.com/ubi10/ubi-micro:10.0-1752626510@sha256:a42817394eb48392dac6ef1677e8554ae290753f4c5048fdb571b369960d5644

# Build stage: build on the host platform for the target platform
FROM --platform=${BUILDPLATFORM} golang:1.24@sha256:a98400bb38a9bd6de5a30c80dc9e2f4969510516cef343bb637f4dcdab71f20b AS builder
WORKDIR /workspace

ARG TARGETPLATFORM
ARG BUILDPLATFORM
ARG TARGETOS
ARG TARGETARCH

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY cmd/main.go cmd/main.go
COPY api/ api/
COPY internal/ internal/

# Build the binary for the target platform
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o manager cmd/main.go

FROM ${BASE_IMAGE}
WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
