ARG BASE_IMAGE=registry.access.redhat.com/ubi10/ubi-micro:10.1-1763138307@sha256:fc8e116afac12a9164be775e21483da7b6e86f91d8e9e6e79dee23afe4e58468

FROM golang:1.25.5@sha256:a22b2e6c5e753345b9759fba9e5c1731ebe28af506745e98f406cc85d50c828e AS builder
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
