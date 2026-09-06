#!/usr/bin/env bash

set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
repo_root=""

template_repository=""
from_commit=""
to_commit=""
dry_run=false
report_file=""

usage() {
  cat <<'EOF'
Usage:
  scripts/template-update.sh --template-repository URL --from-commit SHA --to-commit SHA

Options:
  --template-repository URL  Public or authenticated Git repository URL (required)
  --from-commit SHA           Previous template commit recorded by the project (required)
  --to-commit SHA             Target template commit to apply (required)
  --project-root PATH         Derived repository root (defaults to the script's repository)
  --dry-run                   Report the update without modifying the derived repository
  --report-file PATH          Write the update report to PATH
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
    --dry-run)
      dry_run=true
      shift
      ;;
    --report-file)
      [[ $# -ge 2 ]] || { echo "--report-file requires a value" >&2; exit 2; }
      report_file=$2
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

for command in cmp git jq go; do
  if ! command -v "$command" >/dev/null 2>&1; then
    echo "required command is missing: ${command}" >&2
    exit 1
  fi
done

if [[ -n "$(git -C "$repo_root" status --porcelain)" ]]; then
  echo "the derived repository must have a clean working tree before updating" >&2
  exit 1
fi

validate_ownership_file() {
  local ownership_file=$1
  jq -e '
    .schema_version == 1 and
    ((.template_managed_paths | type) == "array") and
    ((.application_owned_paths | type) == "array") and
    all(.template_managed_paths[]; type == "string" and length > 0) and
    all(.application_owned_paths[];
      type == "string" and length > 0 and
      (startswith("/") | not) and
      (contains("..") | not) and
      (contains("*") | not)
    )
  ' "$ownership_file" >/dev/null || {
    echo "template ownership file is invalid: ${ownership_file}" >&2
    exit 1
  }
}

application_owned_paths=()
add_application_owned_path() {
  local path=$1
  [[ -n "$path" ]] || return 0
  [[ "$path" != /* && "$path" != ../* && "$path" != */../* && "$path" != *'*'* ]] || {
    echo "application-owned paths must be safe relative paths: ${path}" >&2
    exit 1
  }
  local existing
  for existing in "${application_owned_paths[@]}"; do
    [[ "$existing" != "$path" ]] || return 0
  done
  application_owned_paths+=("$path")
}

load_application_owned_paths() {
  local ownership_file=$1
  [[ -f "$ownership_file" ]] || return 0
  validate_ownership_file "$ownership_file"
  while IFS= read -r path; do
    add_application_owned_path "$path"
  done < <(jq -r '.application_owned_paths[]' "$ownership_file")
}

current_manifest="${repo_root}/.template/manifest.json"
if [[ ! -f "$current_manifest" ]]; then
  echo "derived template manifest is missing: .template/manifest.json" >&2
  exit 1
fi
load_application_owned_paths "${repo_root}/.template/ownership.json"
add_application_owned_path "internal/app/routes.go"

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

temporary=$(mktemp -d /tmp/tsc-template-update.XXXXXX)
trap 'rm -rf -- "$temporary"' EXIT
source_dir="${temporary}/template"
target_manifest="${temporary}/target-manifest.json"
patch_file="${temporary}/template.patch"

if [[ -z "$report_file" ]]; then
  report_file="${temporary}/template-update-report.md"
elif [[ "$report_file" != /* ]]; then
  report_file="${repo_root}/${report_file}"
fi
report_directory=$(dirname -- "$report_file")
mkdir -p "$report_directory"

git clone --quiet --no-checkout "$template_repository" "$source_dir"
git -C "$source_dir" fetch --quiet --no-tags origin "$from_commit" "$to_commit"
git -C "$source_dir" cat-file -e "${from_commit}^{commit}"
git -C "$source_dir" cat-file -e "${to_commit}^{commit}"
if ! git -C "$source_dir" merge-base --is-ancestor "$from_commit" "$to_commit"; then
  echo "target template commit is not a descendant of the recorded commit" >&2
  exit 1
fi
git -C "$source_dir" checkout --quiet --detach "$to_commit"

load_application_owned_paths "${source_dir}/.template/ownership.json"

# The generated repository owns these paths. All other paths are eligible for
# template updates, while an ownership file can add more application paths.
template_pathspecs=(
  .
  ':(exclude).template/manifest.json'
)
for path in "${application_owned_paths[@]}"; do
  if [[ -e "${repo_root}/${path}" ]]; then
    template_pathspecs+=(":(exclude)${path}")
  fi
done

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
source_dependency_versions=$(jq -cS -er '.dependency_versions' "$target_manifest")
current_dependency_versions=$(jq -cS -er '.dependency_versions' "$current_manifest")
manifest_dependency_update_required=false
if [[ "$source_dependency_versions" != "$current_dependency_versions" ]]; then
  manifest_dependency_update_required=true
fi

if [[ "$source_go_compatibility" != "$current_go_compatibility" || "$source_postgresql_compatibility" != "$current_postgresql_compatibility" ]]; then
  echo "template compatibility changed; manual migration is required" >&2
  exit 1
fi

if [[ -x "${source_dir}/scripts/validate-template.sh" ]]; then
  source_cache="${temporary}/source-cache"
  (cd "$source_dir" && GOCACHE="$source_cache" ./scripts/validate-template.sh)
fi

breaking_change=false
release_notes=()
breaking_releases=()
while IFS= read -r -d '' release_path; do
  release_notes+=("$release_path")
  release_file="${source_dir}/${release_path}"
  marker=$(sed -n 's/^breaking:[[:space:]]*\(true\|false\)[[:space:]]*$/\1/p' "$release_file" | head -n 1)
  if [[ -z "$marker" ]]; then
    echo "release notes must declare breaking: true|false: ${release_path}" >&2
    exit 1
  fi
  if [[ "$marker" == true ]]; then
    breaking_change=true
    breaking_releases+=("$release_path")
  fi
done < <(git -C "$source_dir" diff --name-only -z "$from_commit" "$to_commit" -- 'docs/releases/*.md')

write_report() {
  local status=${1:-ready}
  local paths=${2:-}
  {
    echo "# Template update report"
    echo
    echo "- status: ${status}"
    echo "- from_commit: ${from_commit}"
    echo "- to_commit: ${to_commit}"
    echo "- template_version: ${template_version}"
    echo "- breaking: ${breaking_change}"
    echo "- dry_run: ${dry_run}"
    echo
    echo "## Release notes"
    if [[ -n "${release_notes[*]:-}" ]]; then
      printf '%s\n' "${release_notes[@]}"
      for release_path in "${release_notes[@]}"; do
        echo
        echo "### ${release_path}"
        sed -n '1,160p' "${source_dir}/${release_path}"
      done
    else
      echo "No release notes changed."
    fi
    if [[ -n "${breaking_releases[*]:-}" ]]; then
      echo
      echo "Breaking release notes:"
      printf '%s\n' "${breaking_releases[@]}"
    fi
    echo
    echo "## Changed template paths"
    if [[ -n "$paths" ]]; then
      printf '%s\n' "$paths"
    else
      echo "No applicable template paths."
    fi
  } > "$report_file"
}

deleted_files=$(git -C "$source_dir" diff --diff-filter=D --name-only "$from_commit" "$to_commit" -- "${template_pathspecs[@]}")
if [[ -n "$deleted_files" ]]; then
  write_report "rejected-deletion" "$deleted_files"
  echo "template updates that delete files require manual migration:" >&2
  printf '%s\n' "$deleted_files" >&2
  exit 1
fi

already_applied_paths=()
normalize_for_comparison() {
	local path=$1
	local input=$2
	local output=$3

	if [[ "$path" == "go.mod" || "$path" == *.go ]]; then
		awk -v source_module="$source_module_path" -v target_module="$target_module_path" '
      function replace_literal(value, source, target, position) {
        while ((position = index(value, source)) > 0) {
          value = substr(value, 1, position - 1) target substr(value, position + length(source))
        }
        return value
      }

      {
        print replace_literal($0, source_module, target_module)
      }
    ' "$input" > "$output"
	elif [[ "$path" == *.yaml || "$path" == *.yml ]]; then
		sed -e 's/\r$//' -e '/^[[:space:]]*$/d' -e 's/[[:space:]]*$//' "$input" > "$output"
	else
		sed 's/\r$//' "$input" > "$output"
	fi
}

while IFS= read -r -d '' path; do
	current_file="${repo_root}/${path}"
	source_file="${source_dir}/${path}"
	current_comparison=$(mktemp "${temporary}/current-comparison.XXXXXX")
	source_comparison=$(mktemp "${temporary}/source-comparison.XXXXXX")
	if [[ -f "$current_file" && -f "$source_file" ]]; then
		normalize_for_comparison "$path" "$current_file" "$current_comparison"
		normalize_for_comparison "$path" "$source_file" "$source_comparison"
	fi
	if [[ -f "$current_file" && -f "$source_file" ]] && cmp -s "$current_comparison" "$source_comparison"; then
		already_applied_paths+=("$path")
		echo "Skipped already-applied template file: ${path}."
	fi
done < <(git -C "$source_dir" diff --name-only -z "$from_commit" "$to_commit" -- "${template_pathspecs[@]}")

patch_pathspecs=("${template_pathspecs[@]}")
for path in "${already_applied_paths[@]}"; do
	patch_pathspecs+=(":(exclude)${path}")
done

git -C "$source_dir" diff --binary --find-renames "$from_commit" "$to_commit" -- "${patch_pathspecs[@]}" > "$patch_file"
changed_paths=$(git -C "$source_dir" diff --name-only "$from_commit" "$to_commit" -- "${patch_pathspecs[@]}" | sort)
if [[ "$manifest_dependency_update_required" == true ]]; then
  changed_paths=$(printf '%s\n%s\n' "$changed_paths" '.template/manifest.json' | sed '/^$/d' | sort -u)
fi
if [[ "$dry_run" == true ]]; then
  write_report "dry-run" "$changed_paths"
  echo "Template update dry-run completed from ${from_commit} to ${to_commit} (version ${template_version})."
  echo "Report: ${report_file}"
  exit 0
fi
if [[ -s "$patch_file" ]]; then
	while IFS= read -r -d '' path; do
		if git -C "$source_dir" cat-file -e "${from_commit}:${path}" 2>/dev/null; then
			git -C "$source_dir" show "${from_commit}:${path}" \
				| git -C "$repo_root" hash-object -w --stdin >/dev/null
		fi
	done < <(git -C "$source_dir" diff --name-only -z "$from_commit" "$to_commit" -- "${patch_pathspecs[@]}")

	direct_patch_file="$patch_file"
	merge_patch_file="$patch_file"
	if [[ "$source_module_path" != "$target_module_path" ]]; then
		direct_patch_file="${temporary}/template-direct.patch"
		merge_patch_file="${temporary}/template-merge.patch"
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
    ' "$patch_file" > "$direct_patch_file"
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
        if ((current_path == "go.mod" || current_path ~ /\.go$/) && $0 ~ /^\+/ && $0 !~ /^\+\+\+ /) {
          line_prefix = substr($0, 1, 1)
          line_body = substr($0, 2)
          $0 = line_prefix replace_literal(line_body, source_module, target_module)
        }
        print
      }
    ' "$patch_file" > "$merge_patch_file"
		echo "Normalized template module paths in Go sources and go.mod to ${target_module_path}."
	fi

	apply_log="${temporary}/apply.log"
	check_log="${temporary}/check.log"
	set +e
	(cd "$repo_root" && git apply --check "$direct_patch_file") >"$check_log" 2>&1
	check_status=$?
	set -e
	if [[ "$check_status" -eq 0 ]]; then
		set +e
		(cd "$repo_root" && git apply "$direct_patch_file") >"$apply_log" 2>&1
		apply_status=$?
		set -e
		applied_with_index=false
	else
		printf '%s\n' "Direct template patch did not apply; trying three-way merge." >"$apply_log"
		set +e
		(cd "$repo_root" && git apply --3way --index "$merge_patch_file") >>"$apply_log" 2>&1
		apply_status=$?
		set -e
		applied_with_index=true
	fi
	cat "$apply_log"
	failed_paths=$(sed -n \
		-e 's/^error: patch failed: \([^:]*\):.*$/\1/p' \
		-e 's/^error: \([^:]*\): patch does not apply$/\1/p' \
		"$apply_log" | sort -u)
	conflict_paths=$(git -C "$repo_root" diff --name-only --diff-filter=U)
	if [[ "$apply_status" -ne 0 || -n "$failed_paths" || -n "$conflict_paths" ]]; then
		reported_paths=$(printf '%s\n%s\n' "$failed_paths" "$conflict_paths" | sed '/^$/d' | sort -u)
		write_report "conflict" "$reported_paths"
		if [[ -n "$reported_paths" ]]; then
			echo "Template update has unresolved or unapplied changes in:" >&2
			printf '%s\n' "$reported_paths" >&2
			echo "Resolve these files manually, stage them, and rerun template provenance validation." >&2
		else
			echo "Template update could not be applied cleanly. Review the working tree before retrying." >&2
		fi
		if [[ "$apply_status" -eq 0 ]]; then
			exit 1
		fi
		exit "$apply_status"
	fi
	if [[ "$applied_with_index" == true ]]; then
		(cd "$repo_root" && git reset --quiet)
	fi
fi

if [[ "$manifest_dependency_update_required" == true ]]; then
  synced_manifest=$(mktemp "${temporary}/synced-manifest.XXXXXX")
  jq --slurpfile source_manifest "$target_manifest" \
    '.dependency_versions = $source_manifest[0].dependency_versions' \
    "$current_manifest" > "$synced_manifest"
  mv "$synced_manifest" "$current_manifest"
  echo "Synchronized dependency versions in .template/manifest.json."
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

write_report "applied" "$changed_paths"

echo "Template update applied from ${from_commit} to ${to_commit} (version ${template_version})."
echo "Review the diff, run the derived repository test suite, and resolve any application-specific conflicts manually."
echo "Report: ${report_file}"
