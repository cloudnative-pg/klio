#!/bin/bash
set -euo pipefail

# renovate: datasource=github-releases depName=go-task/task versioning=semver
TASK_VERSION='3.50.0'
# renovate: datasource=github-releases depName=dagger/dagger versioning=semver
export DAGGER_VERSION='0.20.7'

# Install Task (https://taskfile.dev)
sh -c "$(curl --proto "=https" --tlsv1.2 -sSf -L https://taskfile.dev/install.sh)" -- -d -b /usr/local/bin "v${TASK_VERSION}"

# Install Dagger (https://docs.dagger.io)
curl --proto "=https" --tlsv1.2 -sSf -L https://dl.dagger.io/dagger/install.sh | BIN_DIR=/usr/local/bin sh

# Run Renovate
renovate
