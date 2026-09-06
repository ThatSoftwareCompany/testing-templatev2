#!/usr/bin/env bash

set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
repo_root=""
template_repository=""
to_commit=""
dry_run=false

usage() {
  cat <<'EOF'
Usage:
  scripts/template-update-bootstrap.sh --template-repository URL --to-commit SHA

Options:
  --template-repository URL  Template repository URL (required)
  --to-commit SHA             Target template commit (required)
  --project-root PATH         Derived repository root (defaults to the script's repository)
  --dry-run                   Report changes without modifying the manifest
  -h, --help                  Show this help
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --template-repository)
      [[ $# -ge 2 ]] || { echo "--template-repository requires a value" >&2; exit 2; }
      template_repository=$2
      shift 2
      ;;
    --to-commit)
      [[ $# -ge 2 ]] || { echo "--to-commit requires a value" >&2; exit 2; }
      to_commit=$2
      shift 2
      ;;
    --project-root)
      [[ $# -ge 2 ]] || { echo "--project-root requires a value" >&2; exit 2; }
      repo_root=$2
      shift 2
      ;;
    --dry-run)
      dry_run=true
      shift
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

if [[ -z "$repo_root" ]]; then
  repo_root=$(cd -- "${script_dir}/.." && pwd)
else
  repo_root=$(cd -- "$repo_root" && pwd)
fi

if [[ -z "$template_repository" || -z "$to_commit" ]]; then
  echo "--template-repository and --to-commit are required" >&2
  usage >&2
  exit 2
fi
if [[ ! "$to_commit" =~ ^[0-9a-f]{40}$ ]]; then
  echo "to commit must be a 40-character lowercase Git SHA" >&2
  exit 2
fi

for command in cmp git jq mktemp; do
  if ! command -v "$command" >/dev/null 2>&1; then
    echo "required command is missing: ${command}" >&2
    exit 1
  fi
done

current_manifest="${repo_root}/.template/manifest.json"
if [[ ! -f "$current_manifest" ]]; then
  echo "derived template manifest is missing: .template/manifest.json" >&2
  exit 1
fi
if [[ -n "$(git -C "$repo_root" status --porcelain)" ]]; then
  echo "the derived repository must have a clean working tree before bootstrapping" >&2
  exit 1
fi

temporary=$(mktemp -d /tmp/tsc-template-bootstrap.XXXXXX)
trap 'rm -rf -- "$temporary"' EXIT
source_dir="${temporary}/template"
target_manifest="${temporary}/target-manifest.json"
git clone --quiet --no-checkout "$template_repository" "$source_dir"
git -C "$source_dir" fetch --quiet --no-tags origin "$to_commit"
git -C "$source_dir" cat-file -e "${to_commit}^{commit}"
git -C "$source_dir" show "${to_commit}:.template/manifest.json" > "$target_manifest"

jq -e '.template_version and .dependency_versions and .compatibility and .update_policy' \
  "$target_manifest" >/dev/null || {
  echo "target template manifest is incomplete" >&2
  exit 1
}

synced_manifest=$(mktemp "${temporary}/synced-manifest.XXXXXX")
jq --slurpfile source_manifest "$target_manifest" '
  .template_id = $source_manifest[0].template_id |
  .template_source = $source_manifest[0].template_source |
  .repository = $source_manifest[0].repository |
  .license = $source_manifest[0].license |
  .minimum_go_version = $source_manifest[0].minimum_go_version |
  .database = $source_manifest[0].database |
  .migration_tool = $source_manifest[0].migration_tool |
  .architecture = $source_manifest[0].architecture |
  .supported_environments = $source_manifest[0].supported_environments |
  .dependency_versions = $source_manifest[0].dependency_versions |
  .compatibility = $source_manifest[0].compatibility |
  .update_policy = $source_manifest[0].update_policy
' "$current_manifest" > "$synced_manifest"

if cmp -s "$current_manifest" "$synced_manifest"; then
  echo "Template manifest metadata is already synchronized."
  exit 0
fi

if [[ "$dry_run" == true ]]; then
  echo "Template manifest bootstrap would update managed metadata."
  exit 0
fi

mv "$synced_manifest" "$current_manifest"
echo "Synchronized template manifest metadata without changing provenance or generated project fields."
