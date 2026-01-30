#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/repo.sh"

ROOT_DIR="$(repo_root)"

mkdir -p "$ROOT_DIR/bin"
go build -o "$ROOT_DIR/bin" -v "$ROOT_DIR/..."
