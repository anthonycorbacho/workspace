#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

common::check_installed() {
  local cmd=$1
  local errMsg=$2
  if [[ -z "$(command -v "${cmd}")" ]]; then
      echo -e "${errMsg}"
      return 2
  fi
}

go::verify() {
  common::check_installed "go" "Can't find 'go' in PATH, please fix and retry. See http://golang.org/doc/install for installation instructions."

  local go_version
  local minimum_go_version
  FS=" " read -ra go_version <<< "$(GOFLAGS='' go version)"
  minimum_go_version=go1.18.3

  if [[ "${minimum_go_version}" != $(echo -e "${minimum_go_version}\n${go_version[2]}" | sort -s -t. -k 1,1 -k 2,2n -k 3,3n | head -n1) && "${go_version[2]}" != "devel" ]]; then
    echo -e "Detected go version: ${go_version[*]}."
    echo -e "Cloud-workspace requires ${minimum_go_version} or greater."
    echo -e "Please install ${minimum_go_version} or later."
    retrun 2
  fi
}

go:install_dependencies() {
  common::check_installed "protoc-gen-grpc-gateway" "Can't find 'protoc-gen-grpc-gateway', please run 'go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@latest'"
  common::check_installed "protoc-gen-openapiv2" "Can't find 'protoc-gen-openapiv2', please run 'go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2@latest'"
  common::check_installed "protoc-gen-go" "Can't find 'protoc-gen-go', please run 'go install google.golang.org/protobuf/cmd/protoc-gen-go@latest'"
  common::check_installed "protoc-gen-go-grpc" "Can't find 'protoc-gen-go-grpc', please run 'go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest'"
  common::check_installed "protoc-gen-validate" "Can't find 'protoc-gen-validate', please run 'go install github.com/envoyproxy/protoc-gen-validate@latest'"
}

brew::verify() {
  common::check_installed "brew" "Can't find 'brew' in PATH, please fix and retry. See https://docs.brew.sh/Installation for installation instructions."
}

tilt::verify() {
  common::check_installed "tilt" "Can't find 'tilt' in PATH, please fix and retry. See https://docs.tilt.dev/install.html for installation instructions."
}

kubectl::verify() {
  common::check_installed "kubectl" "Can't find 'kubectl' in PATH, please fix and retry. See https://kubernetes.io/docs/tasks/tools/install-kubectl-macos for installation instructions."
}

kustomize::verify() {
  common::check_installed "kustomize" "Can't find 'kustomize' in PATH, please fix and retry. See https://kubectl.docs.kubernetes.io/installation/kustomize for installation instructions."
}

buf::verify() {
  common::check_installed "buf" "Can't find 'buf' in PATH, please fix and retry. See https://docs.buf.build/installation for installation instructions."
}

golangci::verify() {
  common::check_installed "golangci-lint" "Can't find 'golangci-lint' in PATH, please fix and retry. See https://golangci-lint.run/usage/install/#macos for installation instructions."
}

kubeseal::verify() {
  common::check_installed "kubeseal" "Can't find 'kubeseal' in PATH, please fix and retry. See https://github.com/bitnami-labs/sealed-secrets#homebrew for installation instructions."
}

dockercompose::verify() {
  common::check_installed "docker-compose" "Can't find 'docker-compose' in PATH, please fix and retry. See https://formulae.brew.sh/formula/docker-compose for installation instructions."
}

rancherdesktop::verify() {
  common::check_installed "docker" "Can't find 'rancher-desktop' in PATH, please fix and retry. See https://docs.rancherdesktop.io/getting-started/installation/ for installation instructions."
}

mockery::verify() {
  common::check_installed "mockery" "Can't find 'mockery' in PATH, please fix and retry. See https://github.com/vektra/mockery#installation/ for installation instructions."
}

ko::verify() {
  # use for tilt go container image
  common::check_installed "ko" "Can't find 'ko' in PATH, please fix and retry. \nRun 'brew install ko' \nor see https://github.com/google/ko for installation instructions."
}

brew::verify
rancherdesktop::verify
mockery::verify
tilt::verify
kubectl::verify
kustomize::verify
buf::verify
golangci::verify
kubeseal::verify
dockercompose::verify
go::verify
go:install_dependencies
ko::verify
