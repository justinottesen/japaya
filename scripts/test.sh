#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/repo.sh"

ROOT_DIR="$(repo_root)"

go test "$ROOT_DIR/..." -race -count=1 "$@"
