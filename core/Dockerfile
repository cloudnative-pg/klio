ARG BASE_IMAGE=registry.access.redhat.com/ubi10/ubi-micro:10.0-1754556444@sha256:2a820c70ceee2588618f4b1b6116eaa38952b455e1f9fd3a91f7d8afb21c0873

FROM golang:1.25.4@sha256:6ca9eb0b32a4bd4e8c98a4a2edf2d7c96f3ea6db6eb4fc254eef6c067cf73bb4 AS builder
RUN cd / && \
    git clone https://github.com/leonardoce/kopia && \
    cd kopia && \
    git checkout mtls && \
    go build -o kopia main.go

FROM ${BASE_IMAGE} AS base
ARG TARGETARCH
ARG TARGETOS
COPY --from=builder /kopia/kopia /usr/bin/kopia
COPY dist/klio_${TARGETOS}_${TARGETARCH}*/klio /usr/bin/klio
COPY licenses /licenses
ENTRYPOINT ["/usr/bin/klio"]
USER 26
