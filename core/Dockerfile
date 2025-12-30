ARG BASE_IMAGE=registry.access.redhat.com/ubi10/ubi-micro:10.1-1766049088@sha256:2946fa1b951addbcd548ef59193dc0af9b3e9fedb0287b4ddb6e697b06581622

FROM golang:1.25.5@sha256:31c1e53dfc1cc2d269deec9c83f58729fa3c53dc9a576f6426109d1e319e9e9a AS builder
RUN cd / && \
    git clone https://github.com/leonardoce/kopia && \
    cd kopia && \
    git checkout klio && \
    go build -o kopia main.go

FROM nats:2.12.3-scratch@sha256:30c913e575318c477aa3a81da3f71a00a518e7091d6219d75c8b157e5844f92c AS nats_extractor

FROM ${BASE_IMAGE} AS base
ARG TARGETARCH
ARG TARGETOS
COPY --from=builder /kopia/kopia /usr/bin/kopia
COPY --from=nats_extractor /nats-server /usr/bin/nats-server
COPY dist/klio_${TARGETOS}_${TARGETARCH}*/klio /usr/bin/klio
COPY licenses /licenses
ENTRYPOINT ["/usr/bin/klio"]
USER 26
