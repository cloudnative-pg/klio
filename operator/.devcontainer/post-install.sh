#!/bin/bash
##
## Copyright © contributors to CloudNativePG, established as
## CloudNativePG a Series of LF Projects, LLC.
##
## Licensed under the Apache License, Version 2.0 (the "License");
## you may not use this file except in compliance with the License.
## You may obtain a copy of the License at
##
##     http://www.apache.org/licenses/LICENSE-2.0
##
## Unless required by applicable law or agreed to in writing, software
## distributed under the License is distributed on an "AS IS" BASIS,
## WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
## See the License for the specific language governing permissions and
## limitations under the License.
##
## SPDX-License-Identifier: Apache-2.0
##
set -x

curl --proto "=https" -Lo ./kind https://kind.sigs.k8s.io/dl/latest/kind-linux-amd64
chmod +x ./kind
mv ./kind /usr/local/bin/kind

curl --proto "=https" -L -o kubebuilder https://go.kubebuilder.io/dl/latest/linux/amd64
chmod +x kubebuilder
mv kubebuilder /usr/local/bin/

KUBECTL_VERSION=$(curl --proto "=https" -L -s https://dl.k8s.io/release/stable.txt)
curl --proto "=https" -LO "https://dl.k8s.io/release/$KUBECTL_VERSION/bin/linux/amd64/kubectl"
chmod +x kubectl
mv kubectl /usr/local/bin/kubectl

docker network create -d=bridge --subnet=172.19.0.0/24 kind

kind version
kubebuilder version
docker --version
go version
kubectl version --client
