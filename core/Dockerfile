ARG BASE_IMAGE=registry.access.redhat.com/ubi10/ubi-micro:10.0-1752626510@sha256:a42817394eb48392dac6ef1677e8554ae290753f4c5048fdb571b369960d5644

FROM kopia/kopia:0.21 AS kopia

FROM ${BASE_IMAGE} AS base
ARG TARGETARCH
ARG TARGETOS
COPY --from=kopia /bin/kopia /usr/bin/kopia
COPY dist/klio_${TARGETOS}_${TARGETARCH}*/klio /usr/bin/klio
COPY licenses /licenses
ENTRYPOINT ["/usr/bin/klio"]
USER 26
