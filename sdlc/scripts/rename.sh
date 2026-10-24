#!/usr/bin/env bash
#
# Renames the Go module of the workspace, to start a new repository from this template.
#
#   ./sdlc/scripts/rename.sh github.com/acme/platform
#
# It replaces the module path in the Go files, the go.mod files, the protos (go_package), the linter
# configuration (depguard) and the documentation. When both modules are hosted on GitHub, the
# repository (badges, clone URL) and the owner (CODEOWNERS, packages link) are replaced too.
# The generated protobuf code is then regenerated (buf) since it embeds the go_package.
#
# Run it from a clean working tree to review the changes with `git diff`.

set -euo pipefail

usage() {
  echo "usage: $0 <new module path>   (eg: github.com/acme/platform)" >&2
  exit 2
}

[[ $# -eq 1 ]] || usage
new_module="$1"
[[ "$new_module" =~ ^[a-z0-9.-]+(/[A-Za-z0-9._~-]+)+$ ]] || {
  echo "invalid module path: $new_module" >&2
  usage
}

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$root"

old_module="$(awk '$1 == "module" { print $2; exit }' go.mod)"
[[ -n "$old_module" ]] || { echo "no module path found in $root/go.mod" >&2; exit 1; }
if [[ "$old_module" == "$new_module" ]]; then
  echo "the module is already $new_module"
  exit 0
fi

for tool in go buf perl; do
  command -v "$tool" >/dev/null 2>&1 || { echo "$tool is required (see make onboarding)" >&2; exit 1; }
done

# files lists the text files referencing $1, the vendored and generated protobuf code excluded.
files() {
  grep -rIlF --exclude-dir=vendor --exclude-dir=.git --exclude-dir=node_modules \
    --exclude='*.pb.go' --exclude='*.pb.gw.go' -e "$1" . || true
}

# replace replaces the literal $1 by $2 in the files referencing $1.
replace() {
  local f
  while IFS= read -r f; do
    OLD="$1" NEW="$2" perl -pi -e 's/\Q$ENV{OLD}\E/$ENV{NEW}/g' "$f"
  done < <(files "$1")
}

echo "renaming $old_module to $new_module"
replace "$old_module" "$new_module"

# github.com/<owner>/<repo>: the repository and the owner are referenced outside of the module path.
if [[ "$old_module" =~ ^github\.com/([^/]+)/([^/]+)$ ]]; then
  old_owner="${BASH_REMATCH[1]}" old_repo="${BASH_REMATCH[1]}/${BASH_REMATCH[2]}"
  if [[ "$new_module" =~ ^github\.com/([^/]+)/([^/]+)$ ]]; then
    new_owner="${BASH_REMATCH[1]}" new_repo="${BASH_REMATCH[1]}/${BASH_REMATCH[2]}"
    replace "$old_repo" "$new_repo"
    replace "@$old_owner" "@$new_owner"
    replace "github.com/$old_owner?" "github.com/$new_owner?"
  else
    echo "warning: $new_module is not a github.com/<owner>/<repo> module, update the README badges and .github/CODEOWNERS" >&2
  fi
fi

echo "regenerating the protobuf code"
make -s proto-gen

echo "formatting the imports"
files "$new_module" | grep '\.go$' | xargs gofmt -w

echo "checking the workspace builds"
go build ./...
GOWORK=off go -C sdlc/codegen build ./...

remaining="$(files "$old_module")"
if [[ -n "$remaining" ]]; then
  echo "warning: $old_module is still referenced in:" >&2
  echo "$remaining" >&2
fi

echo "done, review the changes (git diff) then run make lint test"
