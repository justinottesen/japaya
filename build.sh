#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN_DIR="${ROOT_DIR}/bin"

mkdir -p "${BIN_DIR}"

# Hard-code your builds here (paths are relative to the repo root/script dir)
go build -o "${BIN_DIR}/japaya" "${ROOT_DIR}/cmd/japaya"
