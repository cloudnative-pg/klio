ARG BASE_IMAGE="redhat/ubi9-micro:latest"
FROM kopia/kopia:latest AS kopia

FROM ${BASE_IMAGE} AS base
ARG TARGETARCH
ARG TARGETOS
COPY --from=kopia /bin/kopia /kopia
COPY dist/klio/klio_${TARGETOS}_${TARGETARCH} /klio
USER 26
