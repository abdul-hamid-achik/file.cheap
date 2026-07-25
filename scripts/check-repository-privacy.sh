#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repository_root="${1:-$(cd -- "$script_dir/.." && pwd)}"

if ! git -C "$repository_root" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  echo "Privacy check failed: target is not a Git worktree." >&2
  exit 2
fi

# Build these terms from fragments so the checker does not exempt its own
# implementation from the tracked-content policy it enforces.
forbidden_tool='Graph''ite'
forbidden_ticket='[O][P][G]-[0-9]+'
forbidden_phrase='Internal'' Migrant'
forbidden_symbol='INTEL_''Workers_ITA_International'
forbidden_pattern="(${forbidden_tool})|(${forbidden_ticket})|(${forbidden_phrase})|(${forbidden_symbol})"
violations=0

shopt -s nocasematch
while IFS= read -r -d '' tracked_path; do
  if [[ "$tracked_path" =~ $forbidden_pattern ]]; then
    printf 'Forbidden workplace marker in tracked path: %s\n' "$tracked_path" >&2
    violations=1
  fi
done < <(git -C "$repository_root" ls-files -z)
shopt -u nocasematch

content_status=0
content_matches="$(
  git -C "$repository_root" grep -Il -E -i -- "$forbidden_pattern" -- .
)" || content_status=$?

if (( content_status > 1 )); then
  echo "Privacy check failed while scanning tracked file contents." >&2
  exit "$content_status"
fi

if (( content_status == 0 )); then
  while IFS= read -r tracked_path; do
    [[ -z "$tracked_path" ]] && continue
    printf 'Forbidden workplace marker in tracked content: %s\n' "$tracked_path" >&2
  done <<< "$content_matches"
  violations=1
fi

if (( violations != 0 )); then
  echo "Privacy check failed. Replace workplace-specific references with neutral examples." >&2
  exit 1
fi

echo "Privacy check passed: tracked paths and text contain no prohibited workplace markers."
