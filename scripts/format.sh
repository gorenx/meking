#!/usr/bin/env bash

set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  ./scripts/format.sh [--check]

Options:
  --check   Only check formatting; do not modify files
EOF
}

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

CHECK_ONLY=0
case "${1:-}" in
  --check)
    CHECK_ONLY=1
    ;;
  -h|--help)
    usage
    exit 0
    ;;
  "")
    ;;
  *)
    echo "Unsupported argument: ${1}" >&2
    usage >&2
    exit 2
    ;;
esac

if ! command -v gofmt >/dev/null; then
  echo "required tool not found: gofmt" >&2
  exit 1
fi

cd "${PROJECT_ROOT}"

mapfile -t GO_FILES < <(
  if command -v rg >/dev/null; then
    rg --files -g '*.go'
  else
    find . -type f -name '*.go'
  fi
)

if [ "${#GO_FILES[@]}" -eq 0 ]; then
  echo "no go files found"
  exit 0
fi

if [ "${CHECK_ONLY}" -eq 1 ]; then
  mapfile -t UNFORMATTED_GO < <(gofmt -l "${GO_FILES[@]}")
  if [ "${#UNFORMATTED_GO[@]}" -ne 0 ]; then
    echo "gofmt check failed: the following files are not formatted"
    printf '  %s\n' "${UNFORMATTED_GO[@]}"
    exit 1
  fi
else
  gofmt -w "${GO_FILES[@]}"
fi

if command -v goimports >/dev/null; then
  if [ "${CHECK_ONLY}" -eq 1 ]; then
    mapfile -t UNFORMATTED_GOIMPORTS < <(goimports -l "${GO_FILES[@]}")
    if [ "${#UNFORMATTED_GOIMPORTS[@]}" -ne 0 ]; then
      echo "goimports check failed: the following files are not import-formatted"
      printf '  %s\n' "${UNFORMATTED_GOIMPORTS[@]}"
      exit 1
    fi
  else
    goimports -w "${GO_FILES[@]}"
  fi
else
  if [ "${CHECK_ONLY}" -eq 0 ]; then
    echo "tip: install goimports to align import blocks automatically"
  fi
fi

if [ "${CHECK_ONLY}" -eq 1 ]; then
  echo "format check passed"
else
  echo "formatting done"
fi
