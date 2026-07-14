#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

status_file="${1:-}"
if [[ -z "$status_file" ]]; then
  status_file="$(mktemp -t meshclaw-release-dirty-audit.XXXXXX)"
fi

git status --short > "$status_file"
tracked_changed="$(git status --short | awk '$1 !~ /^\?\?/ {count++} END {print count+0}')"
untracked="$(git status --short | awk '$1 == "??" {count++} END {print count+0}')"
total="$(wc -l < "$status_file" | tr -d ' ')"

echo "MeshClaw release dirty audit"
echo "root: $root"
echo "status_file: $status_file"
echo "total_dirty: $total"
echo "tracked_changed: $tracked_changed"
echo "untracked: $untracked"
echo
echo "Top dirty paths:"
sed -n '1,80p' "$status_file"
echo
echo "Release rule: do not deploy a release build until intentional changes are committed/stashed or explicitly accepted."
