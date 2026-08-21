url = "https://github.com/cloudnative-pg/klio"
authors = "The CloudNativePG Contributors"
title = "Klio Operator Image"
description = "Klio Operator is a Kubernetes operator designed to manage and deploy Klio servers on Kubernetes clusters. It automates the lifecycle of Klio server resources, streamlining deployment, configuration, and management tasks for cloud-native environments."
# TODO: add revision information
revision = ""
documentation = "https://cloudnative-pg.io/klio/"
license = "Apache-2.0"
now = timestamp()

variable "environment" {
  default = "testing"
  validation {
    condition = contains(["testing", "production"], environment)
    error_message = "environment must be either testing or production"
  }
}
suffix = (environment == "testing") ? "-testing" : ""

variable "insecure" {
  default = "false"
}

variable "latest" {
  default = "false"
}

variable "registry" {
  default = "localhost:5000"
}

variable "base_image" {
  // renovate image: datasource=docker depName=static-debian13 lookupName=gcr.io/distroless/static-debian13 versioning=docker
  default = "gcr.io/distroless/static-debian13:nonroot@sha256:f7f8f729987ad0fdf6b05eeeae94b26e6a0f613bdf46feea7fc40f7bd72953e6"
}

variable "ubi_base_image" {
  // renovate image: datasource=docker depName=registry.access.redhat.com/ubi9/ubi-micro versioning=docker
  default = "registry.access.redhat.com/ubi9/ubi-micro:9.8-1786321990@sha256:7e7f79ab747bf2b452e3043dd89f388e92be4c7fdcc8b815b58adf6c99c39c95"
}

// The image variants we build. Each one is a separate target of the "default"
// group, built from the same Dockerfile with a different base image. The
// distroless variant is the primary one and keeps the plain tag; the UBI
// variant is the one submitted to Red Hat certification.
distros = {
  distroless = {
    baseImage = base_image
    tagSuffix = ""
  }
  ubi = {
    baseImage = ubi_base_image
    tagSuffix = "-ubi9"
  }
}

function "getRegistry" {
  params = []
  result = lower(registry)
}

function "getImageName" {
  params = []
  result = "${getRegistry()}/klio-operator${suffix}"
}

variable "version" {
  default = "dev"
}

target "default" {
  matrix = {
    distro = ["distroless", "ubi"]
  }
  name = distro

  dockerfile = "Dockerfile"
  context = "."
  platforms = [
    "linux/amd64",
    "linux/arm64"
  ]

  tags = [
    latest("${getImageName()}", "${latest}", "${distros[distro].tagSuffix}"),
    "${getImageName()}:${version}${distros[distro].tagSuffix}",
  ]

  args = {
    "BASE_IMAGE" = "${distros[distro].baseImage}",
  }

  output = [
    {
      "type" = "image",
      "registry.insecure" = "${insecure}",
    },
  ]

  attest = [
    "type=provenance,mode=max",
    "type=sbom"
  ]

  annotations = [
    "index,manifest:org.opencontainers.image.created=${now}",
    "index,manifest:org.opencontainers.image.url=${url}",
    "index,manifest:org.opencontainers.image.source=${url}",
    "index,manifest:org.opencontainers.image.version=${version}",
    "index,manifest:org.opencontainers.image.revision=${revision}",
    "index,manifest:org.opencontainers.image.vendor=${authors}",
    "index,manifest:org.opencontainers.image.title=${title}",
    "index,manifest:org.opencontainers.image.description=${description}",
    "index,manifest:org.opencontainers.image.documentation=${documentation}",
    "index,manifest:org.opencontainers.image.authors=${authors}",
    "index,manifest:org.opencontainers.image.licenses=${license}",
    "index,manifest:org.opencontainers.image.base.name=${baseName(distros[distro].baseImage)}",
    "index,manifest:org.opencontainers.image.base.digest=${digest(distros[distro].baseImage)}",
  ]
  labels = {
    "org.opencontainers.image.created"       = "${now}",
    "org.opencontainers.image.url"           = "${url}",
    "org.opencontainers.image.source"        = "${url}",
    "org.opencontainers.image.version"       = "${version}",
    "org.opencontainers.image.revision"      = "${revision}",
    "org.opencontainers.image.vendor"        = "${authors}",
    "org.opencontainers.image.title"         = "${title}",
    "org.opencontainers.image.description"   = "${description}",
    "org.opencontainers.image.documentation" = "${documentation}",
    "org.opencontainers.image.authors"       = "${authors}",
    "org.opencontainers.image.licenses"      = "${license}",
    "org.opencontainers.image.base.name"     = "${baseName(distros[distro].baseImage)}",
    "org.opencontainers.image.base.digest"   = "${digest(distros[distro].baseImage)}",
    "name"                                   = "${title}",
    "maintainer"                             = "${authors}",
    "vendor"                                 = "${authors}",
    "version"                                = "${version}",
    "release"                                = "1",
    "description"                            = "${description}",
    "summary"                                = "${description}",
  }
}

// We get the sha256 of the image
function digest {
  params = [ imageNameWithSha ]
  result = index(split("@", imageNameWithSha), 1)
}

// We get the image reference without the sha256, so that the base.name label
// is always derived from the variant's base image and cannot drift away from it.
function baseName {
  params = [ imageNameWithSha ]
  result = index(split("@", imageNameWithSha), 0)
}

// The moving tag of each variant: ":latest" for the primary one and
// ":latest<suffix>" for the others, so the variants never collide on it.
function latest {
  params = [ image, latest, tagSuffix ]
  result = (latest == "true") ? "${image}:latest${tagSuffix}" : ""
}
