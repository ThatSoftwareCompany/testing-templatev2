#!/usr/bin/env bash

set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
repo_root=$(cd -- "${script_dir}/.." && pwd)

workflow_count=0
failure=0
while IFS= read -r -d '' workflow; do
  workflow_count=$((workflow_count + 1))
  line_number=0
  while IFS= read -r line; do
    line_number=$((line_number + 1))
    [[ "$line" =~ ^[[:space:]]*(-[[:space:]]*)?uses:[[:space:]]* ]] || continue

    value=${line#*:}
    value=${value%%#*}
    value=$(printf '%s' "$value" | sed 's/^[[:space:]]*//; s/[[:space:]]*$//')
    [[ -n "$value" ]] || continue
    [[ "$value" == ./* ]] && continue

    if [[ "$value" != *@* ]]; then
      echo "Action reference is not pinned: ${workflow#"$repo_root/"}:${line_number}: ${value}" >&2
      failure=1
      continue
    fi

    reference=${value##*@}
    if [[ ! "$reference" =~ ^[0-9a-f]{40}$ ]]; then
      echo "Action reference must use a 40-character lowercase SHA: ${workflow#"$repo_root/"}:${line_number}: ${value}" >&2
      failure=1
      continue
    fi

    if [[ "$line" != *'# v'* ]]; then
      echo "Pinned action must retain a human-readable version comment: ${workflow#"$repo_root/"}:${line_number}" >&2
      failure=1
    fi
  done < "$workflow"
done < <(find "$repo_root/.github/workflows" -type f \( -name '*.yml' -o -name '*.yaml' \) -print0 | sort -z)

if [[ "$workflow_count" -eq 0 ]]; then
  echo "no GitHub Actions workflows were found" >&2
  exit 1
fi

if [[ "$failure" -ne 0 ]]; then
  exit 1
fi

echo "all GitHub Actions use full SHA pins"
