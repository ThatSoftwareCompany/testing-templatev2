#!/usr/bin/env bash

set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
repo_root=$(cd -- "${script_dir}/.." && pwd)
exceptions_file="${SECURITY_EXCEPTIONS_FILE:-${repo_root}/.github/security-exceptions.json}"
scanner=""
findings_file=""

usage() {
  cat <<'EOF'
Usage:
  scripts/check-security-exceptions.sh --scanner NAME --findings-file PATH [--exceptions-file PATH]

The findings file must contain one exact finding per line:
  finding-id<TAB>component
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --scanner)
      [[ $# -ge 2 ]] || { echo "--scanner requires a value" >&2; exit 2; }
      scanner=$2
      shift 2
      ;;
    --findings-file)
      [[ $# -ge 2 ]] || { echo "--findings-file requires a value" >&2; exit 2; }
      findings_file=$2
      shift 2
      ;;
    --exceptions-file)
      [[ $# -ge 2 ]] || { echo "--exceptions-file requires a value" >&2; exit 2; }
      exceptions_file=$2
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

[[ "$scanner" =~ ^[a-z0-9-]+$ ]] || { echo "a valid --scanner is required" >&2; exit 2; }
[[ -f "$findings_file" ]] || { echo "findings file does not exist: $findings_file" >&2; exit 2; }
command -v jq >/dev/null 2>&1 || { echo "jq is required" >&2; exit 1; }

if [[ ! -f "$exceptions_file" ]]; then
  if grep -q '[^[:space:]]' "$findings_file"; then
    echo "security findings are not allowlisted" >&2
    exit 1
  fi
  exit 0
fi

today=$(date -u +%F)
while IFS=$'\t' read -r finding_id component remainder; do
  [[ -z "${finding_id}${component}${remainder}" ]] && continue
  [[ -n "$finding_id" && -n "$component" && -z "$remainder" ]] || {
    echo "malformed security finding: ${finding_id} ${component} ${remainder}" >&2
    exit 1
  }
  jq -e --arg scanner "$scanner" --arg id "$finding_id" --arg component "$component" --arg today "$today" '
    any(.exceptions[];
      .scanner == $scanner and
      .id == $id and
      .component == $component and
      .expires_on >= $today
    )
  ' "$exceptions_file" >/dev/null || {
    echo "security finding is not covered by a valid exception: ${scanner} ${finding_id} ${component}" >&2
    exit 1
  }
done < "$findings_file"

echo "security findings are covered by explicit, unexpired exceptions"
