ARG BASE_IMAGE="redhat/ubi9-micro:latest@sha256:955512628a9104d74f7b3b0a91db27a6bbecdd6a1975ce0f1b2658d3cd060b98"
FROM kopia/kopia:latest@sha256:0c55a361a353f69a121572920e7af3eb54b014f99df1f2fd8a595adcc33c2904 AS kopia

FROM ${BASE_IMAGE} AS base
ARG TARGETARCH
ARG TARGETOS
COPY --from=kopia /bin/kopia /kopia
COPY dist/klio/klio_${TARGETOS}_${TARGETARCH} /klio
USER 26
