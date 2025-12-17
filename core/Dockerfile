ARG BASE_IMAGE=registry.access.redhat.com/ubi10/ubi-micro:10.1-1765178423@sha256:d5ba6a075880deb061ac2c7d89c7c409e7948b3abd60b591578621c1b6307b62

FROM golang:1.25.5@sha256:36b4f45d2874905b9e8573b783292629bcb346d0a70d8d7150b6df545234818f AS builder
RUN cd / && \
    git clone https://github.com/leonardoce/kopia && \
    cd kopia && \
    git checkout mtls && \
    go build -o kopia main.go

FROM nats:2.12.2-scratch AS nats_extractor

FROM ${BASE_IMAGE} AS base
ARG TARGETARCH
ARG TARGETOS
COPY --from=builder /kopia/kopia /usr/bin/kopia
COPY --from=nats_extractor /nats-server /usr/bin/nats-server
COPY dist/klio_${TARGETOS}_${TARGETARCH}*/klio /usr/bin/klio
COPY licenses /licenses
ENTRYPOINT ["/usr/bin/klio"]
USER 26
