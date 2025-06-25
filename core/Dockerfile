ARG BASE_IMAGE=registry.redhat.io/ubi10/ubi-micro:latest
FROM kopia/kopia:latest@sha256:0c55a361a353f69a121572920e7af3eb54b014f99df1f2fd8a595adcc33c2904 AS kopia

FROM ${BASE_IMAGE} AS base
ARG TARGETARCH
ARG TARGETOS
COPY --from=kopia /bin/kopia /usr/bin/kopia
COPY dist/klio_${TARGETOS}_${TARGETARCH}*/klio /usr/bin/klio
ENTRYPOINT ["/usr/bin/klio"]
USER 26
