# Shell library: repo helpers.
# Intended usage:
#   source "$(dirname "${BASH_SOURCE[0]}")/repo.sh"
#   ROOT_DIR="$(repo_root)"

repo_root() {
  # Find repo root by walking up from the caller script directory.
  # Markers: go.mod or .git
  local start_dir
  start_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[1]}")" && pwd)"

  local d="$start_dir"
  while [[ "$d" != "/" ]]; do
    if [[ -f "$d/go.mod" ]] || [[ -d "$d/.git" ]]; then
      printf '%s\n' "$d"
      return 0
    fi
    d="$(dirname -- "$d")"
  done

  printf 'error: could not find repo root from %s (expected go.mod or .git)\n' "$start_dir" >&2
  return 1
}
