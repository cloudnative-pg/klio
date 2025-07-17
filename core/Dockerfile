ARG BASE_IMAGE=registry.access.redhat.com/ubi10/ubi-micro:10.0-1752626510@sha256:a42817394eb48392dac6ef1677e8554ae290753f4c5048fdb571b369960d5644

FROM golang:1.24-alpine@sha256:68932fa6d4d4059845c8f40ad7e654e626f3ebd3706eef7846f319293ab5cb7a AS kopia
RUN CGO_ENABLED=0 go install github.com/kopia/kopia@a8fc80f47c9875076b1eaaa9ba0a2248a6b5c98d

FROM ${BASE_IMAGE} AS base
ARG TARGETARCH
ARG TARGETOS
COPY --from=kopia /go/bin/kopia /usr/bin/kopia
COPY dist/klio_${TARGETOS}_${TARGETARCH}*/klio /usr/bin/klio
ENTRYPOINT ["/usr/bin/klio"]
USER 26
