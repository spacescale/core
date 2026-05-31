#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
output="$repo_root/context.txt"
tmp="$(mktemp "$repo_root/context.txt.XXXXXX")"

cleanup() {
  rm -f "$tmp"
}
trap cleanup EXIT

find "$repo_root" -type f -name '*.go' -print | sort | while IFS= read -r file; do
  rel="${file#$repo_root/}"
  printf '===== %s =====\n' "$rel"
  cat "$file"
  printf '\n'
done > "$tmp"

mv "$tmp" "$output"
trap - EXIT

count="$(find "$repo_root" -type f -name '*.go' -print | wc -l | tr -d ' ')"
printf 'Wrote %s from %s Go files.\n' "$output" "$count"
