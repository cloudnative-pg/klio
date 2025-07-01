ARG BASE_IMAGE=registry.access.redhat.com/ubi10/ubi-micro:latest

FROM golang:1.24-alpine AS kopia
RUN CGO_ENABLED=0 go install github.com/kopia/kopia@a8fc80f47c9875076b1eaaa9ba0a2248a6b5c98d

FROM ${BASE_IMAGE} AS base
ARG TARGETARCH
ARG TARGETOS
COPY --from=kopia /go/bin/kopia /usr/bin/kopia
COPY dist/klio_${TARGETOS}_${TARGETARCH}*/klio /usr/bin/klio
ENTRYPOINT ["/usr/bin/klio"]
USER 26
