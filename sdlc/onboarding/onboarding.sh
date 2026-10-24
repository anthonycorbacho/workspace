#!/usr/bin/env bash
# Checks that the tools required by the workspace are installed, all the missing tools are reported at once.

set -o nounset
set -o pipefail

missing=0

# check_installed reports a missing command with its installation instructions.
check_installed() {
  local cmd=$1
  local help=$2
  if ! command -v "${cmd}" > /dev/null 2>&1; then
    echo "✗ ${cmd} is missing: ${help}"
    missing=$((missing + 1))
  else
    echo "✓ ${cmd}"
  fi
}

check_go_version() {
  command -v go > /dev/null 2>&1 || return
  local minimum=go1.27.0
  local version
  version=$(GOFLAGS='' go env GOVERSION)
  if [[ "${version}" != devel* && "${minimum}" != "$(printf '%s\n%s\n' "${minimum}" "${version}" | sort -V | head -n1)" ]]; then
    echo "✗ go ${version} is too old, the workspace requires ${minimum} or later"
    missing=$((missing + 1))
  fi
}

check_docker_compose() {
  command -v docker > /dev/null 2>&1 || return
  if ! docker compose version > /dev/null 2>&1; then
    echo "✗ docker compose is missing: install Docker Desktop or the compose plugin (https://docs.docker.com/compose/install/)"
    missing=$((missing + 1))
  fi
}

check_go_tools() {
  command -v go > /dev/null 2>&1 || return
  # the protoc plugins are pinned in go.mod (tool directive) and run with `go tool`.
  if ! GOFLAGS='' go tool -n protoc-gen-go > /dev/null 2>&1; then
    echo "✗ the go tools (protoc plugins) can't be resolved: run 'go mod download'"
    missing=$((missing + 1))
  fi
}

check_installed go "https://go.dev/doc/install (brew install go)"
check_installed docker "Docker Desktop or Rancher Desktop with a local kubernetes cluster (https://docs.docker.com/desktop/)"
check_installed kubectl "https://kubernetes.io/docs/tasks/tools/ (brew install kubectl)"
check_installed helm "https://helm.sh/docs/intro/install/ (brew install helm), used to render the local charts"
check_installed tilt "https://docs.tilt.dev/install.html (brew install tilt)"
check_installed ko "https://ko.build/install/ (brew install ko)"
check_installed buf "https://buf.build/docs/installation (brew install bufbuild/buf/buf)"
check_installed golangci-lint "https://golangci-lint.run/welcome/install/ (brew install golangci-lint)"
check_go_version
check_docker_compose
check_go_tools

if [[ ${missing} -gt 0 ]]; then
  echo
  echo "${missing} requirement(s) missing, see above."
  exit 1
fi
echo
echo "All the requirements are installed."
