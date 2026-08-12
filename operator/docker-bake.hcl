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
  // renovate image: datasource=docker depName=gcr.io/distroless/static-debian13 versioning=docker
  default = "gcr.io/distroless/static-debian13@sha256:9197324ba51d9cd071af8505989365c006adf9d6d2067eada25aef00abbb5278"
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
    "index,manifest:org.opencontainers.image.base.name=gcr.io/distroless/static-debian13",
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
    "org.opencontainers.image.base.name"     = "gcr.io/distroless/static-debian13",
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

function latest {
  params = [ image, latest ]
  result = (latest == "true") ? "${image}:latest" : ""
}
