url = "https://github.com/cloudnative-pg/klio"
authors = "The CloudNativePG Contributors"
title = "Klio"
description = "Single container image for Klio client and server"
# TODO: add revision information, documentation links, and license information
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
  // renovate image: datasource=docker depName=debian versioning=docker
  default = "debian:13.6-slim@sha256:d7e12182ce18b85b93007c1dedf31f2d29e01ccf3182cc4017c709b6259bc132"
}

function "getRegistry" {
  params = []
  result = lower(registry)
}

function "getImageName" {
  params = []
  result = "${getRegistry()}/klio${suffix}"
}

variable "version" {
  default = "dev"
}

target "default" {
  dockerfile = "Dockerfile"
  context = "."
  platforms = [
    "linux/amd64",
    "linux/arm64"
  ]

  tags = [
    latest("${getImageName()}", "${latest}"),
    "${getImageName()}:${version}",
  ]

  args = {
    "BASE_IMAGE" = "${base_image}",
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
    "index,manifest:org.opencontainers.image.base.name=${baseName(base_image)}",
    "index,manifest:org.opencontainers.image.base.digest=${digest(base_image)}",
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
    "org.opencontainers.image.base.name"     = "${baseName(base_image)}",
    "org.opencontainers.image.base.digest"   = "${digest(base_image)}",
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
// is always derived from base_image and cannot drift away from it.
function baseName {
  params = [ imageNameWithSha ]
  result = index(split("@", imageNameWithSha), 0)
}

function latest {
  params = [ image, latest ]
  result = (latest == "true") ? "${image}:latest" : ""
}
