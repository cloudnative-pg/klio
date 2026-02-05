#!/bin/bash
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
