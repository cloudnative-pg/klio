# syntax=docker/dockerfile:1

ARG BASE_IMAGE=registry.access.redhat.com/ubi10/ubi-micro:10.0-1751548774@sha256:d2f137c785e19a98811578e9cdd07dd087c5702c5eaedf4a5b56592d2a2cb986

# Build stage: build on the host platform for the target platform
FROM --platform=${BUILDPLATFORM} golang:1.24@sha256:20a022e5112a144aa7b7aeb3f22ebf2cdaefcc4aac0d64e8deeee8cdc18b9c0f AS builder
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
