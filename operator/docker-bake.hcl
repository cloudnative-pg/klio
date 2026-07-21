url = "https://www.enterprisedb.com"
authors = "EnterpriseDB"
title = "Klio Operator Image"
description = "Klio Operator is a Kubernetes operator designed to manage and deploy Klio servers on Kubernetes clusters. It automates the lifecycle of Klio server resources, streamlining deployment, configuration, and management tasks for cloud-native environments."
# TODO: add revision information, documentation links, and license information
revision = ""
documentation = ""
license = ""
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
  // renovate image: datasource=docker depName=registry.access.redhat.com/ubi10/ubi-micro versioning=docker
  default = "registry.access.redhat.com/ubi10/ubi-micro:10.2-1784668653@sha256:af12ec115e0b0c19432cea2b826dea5b8d0d06f77f7c318aed93915b3233abea"
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
    "index,manifest:org.opencontainers.image.base.name=ubi10/ubi-micro",
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
    "org.opencontainers.image.base.name"     = "ubi10/ubi-micro",
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
