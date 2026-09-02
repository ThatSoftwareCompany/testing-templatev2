#!/usr/bin/env bash

set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
repo_root=""

template_repository=""
from_commit=""
to_commit=""

usage() {
  cat <<'EOF'
Usage:
  scripts/template-update.sh --template-repository URL --from-commit SHA --to-commit SHA

Options:
  --template-repository URL  Public or authenticated Git repository URL (required)
  --from-commit SHA           Previous template commit recorded by the project (required)
  --to-commit SHA             Target template commit to apply (required)
  --project-root PATH         Derived repository root (defaults to the script's repository)
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
    --from-commit)
      [[ $# -ge 2 ]] || { echo "--from-commit requires a value" >&2; exit 2; }
      from_commit=$2
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

if [[ -z "$template_repository" || -z "$from_commit" || -z "$to_commit" ]]; then
  echo "--template-repository, --from-commit, and --to-commit are required" >&2
  usage >&2
  exit 2
fi
if [[ ! "$from_commit" =~ ^[0-9a-f]{40}$ || ! "$to_commit" =~ ^[0-9a-f]{40}$ ]]; then
  echo "commits must be 40-character lowercase Git SHAs" >&2
  exit 2
fi
if [[ "$from_commit" == "$to_commit" ]]; then
  echo "from and to commits must be different" >&2
  exit 2
fi

resolve_tag_commit() {
  local version=$1
  local commit
  commit=$(git ls-remote "$template_repository" "refs/tags/v${version}^{}" 2>/dev/null | awk 'NR == 1 { print $1 }')
  if [[ -z "$commit" ]]; then
    commit=$(git ls-remote "$template_repository" "refs/tags/v${version}" 2>/dev/null | awk 'NR == 1 { print $1 }')
  fi
  printf '%s' "$commit"
}

for command in git jq go; do
  if ! command -v "$command" >/dev/null 2>&1; then
    echo "required command is missing: ${command}" >&2
    exit 1
  fi
done

if [[ -n "$(git -C "$repo_root" status --porcelain)" ]]; then
  echo "the derived repository must have a clean working tree before updating" >&2
  exit 1
fi

current_manifest="${repo_root}/.template/manifest.json"
if [[ ! -f "$current_manifest" ]]; then
  echo "derived template manifest is missing: .template/manifest.json" >&2
  exit 1
fi

current_commit=$(jq -er '.template_commit' "$current_manifest")
current_version=$(jq -er '.template_version' "$current_manifest")
current_release_commit=$(resolve_tag_commit "$current_version")
if [[ "$current_commit" != "$from_commit" && "$current_release_commit" != "$from_commit" ]]; then
  echo "from commit does not match the recorded source commit or the released template version" >&2
  exit 1
fi
if [[ "$current_commit" != "$from_commit" && "$current_release_commit" == "$from_commit" ]]; then
  echo "Using source commit ${from_commit} resolved from template version ${current_version}."
fi

# internal/app/routes.go is an application-owned extension point. Preserve
# derived repository route registrations when the file already exists. Older
# generated repositories must receive the file once during their migration.
template_pathspecs=(
  .
  ':(exclude).template/manifest.json'
)
if [[ -f "${repo_root}/internal/app/routes.go" ]]; then
  template_pathspecs+=(':(exclude)internal/app/routes.go')
fi

temporary=$(mktemp -d /tmp/tsc-template-update.XXXXXX)
trap 'rm -rf -- "$temporary"' EXIT
source_dir="${temporary}/template"
target_manifest="${temporary}/target-manifest.json"
patch_file="${temporary}/template.patch"

git clone --quiet --no-checkout "$template_repository" "$source_dir"
git -C "$source_dir" fetch --quiet --no-tags origin "$from_commit" "$to_commit"
git -C "$source_dir" cat-file -e "${from_commit}^{commit}"
git -C "$source_dir" cat-file -e "${to_commit}^{commit}"
if ! git -C "$source_dir" merge-base --is-ancestor "$from_commit" "$to_commit"; then
  echo "target template commit is not a descendant of the recorded commit" >&2
  exit 1
fi
git -C "$source_dir" checkout --quiet --detach "$to_commit"

target_module_path=$(awk '$1 == "module" { print $2; exit }' "${repo_root}/go.mod")
source_module_path=$(git -C "$source_dir" show "${to_commit}:go.mod" | awk '$1 == "module" { print $2; exit }')
if [[ -z "$target_module_path" || -z "$source_module_path" ]]; then
  echo "unable to resolve Go module paths for template update" >&2
  exit 1
fi

git -C "$source_dir" show "${to_commit}:.template/manifest.json" > "$target_manifest"
template_version=$(jq -er '.template_version' "$target_manifest")
source_go_compatibility=$(jq -er '.compatibility.go' "$target_manifest")
source_postgresql_compatibility=$(jq -er '.compatibility.postgresql' "$target_manifest")
current_go_compatibility=$(jq -er '.compatibility.go' "$current_manifest")
current_postgresql_compatibility=$(jq -er '.compatibility.postgresql' "$current_manifest")

if [[ "$source_go_compatibility" != "$current_go_compatibility" || "$source_postgresql_compatibility" != "$current_postgresql_compatibility" ]]; then
  echo "template compatibility changed; manual migration is required" >&2
  exit 1
fi

if [[ -x "${source_dir}/scripts/validate-template.sh" ]]; then
  source_cache="${temporary}/source-cache"
  (cd "$source_dir" && GOCACHE="$source_cache" ./scripts/validate-template.sh)
fi

deleted_files=$(git -C "$source_dir" diff --diff-filter=D --name-only "$from_commit" "$to_commit" -- "${template_pathspecs[@]}")
if [[ -n "$deleted_files" ]]; then
  echo "template updates that delete files require manual migration:" >&2
  printf '%s\n' "$deleted_files" >&2
  exit 1
fi

git -C "$source_dir" diff --binary --find-renames "$from_commit" "$to_commit" -- "${template_pathspecs[@]}" > "$patch_file"
if [[ -s "$patch_file" ]]; then
  if [[ "$source_module_path" != "$target_module_path" ]]; then
    normalized_patch_file="${temporary}/template-normalized.patch"
    awk -v source_module="$source_module_path" -v target_module="$target_module_path" '
      function replace_literal(value, source, target, position) {
        while ((position = index(value, source)) > 0) {
          value = substr(value, 1, position - 1) target substr(value, position + length(source))
        }
        return value
      }

      /^diff --git / {
        current_path = $0
        sub(/^diff --git a\/[^ ]+ b\//, "", current_path)
      }

      {
        if (current_path == "go.mod" || current_path ~ /\.go$/) {
          $0 = replace_literal($0, source_module, target_module)
        }
        print
      }
    ' "$patch_file" > "$normalized_patch_file"
    patch_file="$normalized_patch_file"
    echo "Normalized template module paths in Go sources and go.mod to ${target_module_path}."
  fi
  (cd "$repo_root" && git apply --3way --index "$patch_file")
  (cd "$repo_root" && git reset --quiet)
fi

go_cache=${GOCACHE:-}
if [[ -z "$go_cache" || ! -d "$go_cache" || ! -w "$go_cache" ]]; then
  go_cache=$(mktemp -d /tmp/tsc-template-derived-cache.XXXXXX)
fi
(cd "$repo_root" && GOCACHE="$go_cache" go run ./cmd/template \
  -command record-provenance \
  -template-version "$template_version" \
  -template-commit "$to_commit")
(cd "$repo_root" && GOCACHE="$go_cache" go run ./cmd/template -command validate)

echo "Template update applied from ${from_commit} to ${to_commit} (version ${template_version})."
echo "Review the diff, run the derived repository test suite, and resolve any application-specific changes manually."
