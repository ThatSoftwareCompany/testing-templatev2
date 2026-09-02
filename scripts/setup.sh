#!/usr/bin/env bash

set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
repo_root=$(cd -- "${script_dir}/.." && pwd)

project_name=""
module_path=""
app_name=""
environment="development"
database_enabled="true"
architecture="modular-mvc"
generated_from=""
template_commit_override=""

usage() {
  cat <<'EOF'
Usage:
  scripts/setup.sh --project-name NAME --module-path MODULE [options]

Options:
  --project-name NAME       Generated project display name (required)
  --module-path MODULE      Go module path (required)
  --app-name NAME           Application name (defaults to a project slug)
  --environment ENV         development, test, or production
  --database-enabled BOOL   true or false
  --architecture NAME       modular-mvc
  --generated-from VALUE    Derived repository identifier (defaults to module path)
  --template-commit SHA     Template source commit override (optional)
  -h, --help                Show this help
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --project-name)
      [[ $# -ge 2 ]] || { echo "--project-name requires a value" >&2; exit 2; }
      project_name=$2
      shift 2
      ;;
    --module-path)
      [[ $# -ge 2 ]] || { echo "--module-path requires a value" >&2; exit 2; }
      module_path=$2
      shift 2
      ;;
    --app-name)
      [[ $# -ge 2 ]] || { echo "--app-name requires a value" >&2; exit 2; }
      app_name=$2
      shift 2
      ;;
    --environment)
      [[ $# -ge 2 ]] || { echo "--environment requires a value" >&2; exit 2; }
      environment=$2
      shift 2
      ;;
    --database-enabled)
      [[ $# -ge 2 ]] || { echo "--database-enabled requires a value" >&2; exit 2; }
      database_enabled=$2
      shift 2
      ;;
    --architecture)
      [[ $# -ge 2 ]] || { echo "--architecture requires a value" >&2; exit 2; }
      architecture=$2
      shift 2
      ;;
    --generated-from)
      [[ $# -ge 2 ]] || { echo "--generated-from requires a value" >&2; exit 2; }
      generated_from=$2
      shift 2
      ;;
    --template-commit)
      [[ $# -ge 2 ]] || { echo "--template-commit requires a value" >&2; exit 2; }
      template_commit_override=$2
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

if [[ -z "$project_name" || -z "$module_path" ]]; then
  echo "--project-name and --module-path are required" >&2
  usage >&2
  exit 2
fi
if [[ ! "$project_name" =~ ^[A-Za-z0-9][A-Za-z0-9._\ \'-]*$ ]]; then
  echo "project name contains unsupported characters" >&2
  exit 2
fi
if [[ ! "$module_path" =~ ^[A-Za-z0-9][A-Za-z0-9._/-]*$ ]]; then
  echo "module path contains unsupported characters" >&2
  exit 2
fi
if [[ -z "$generated_from" ]]; then
  generated_from=$module_path
fi
if [[ ! "$generated_from" =~ ^[A-Za-z0-9][A-Za-z0-9._:/-]*$ ]]; then
  echo "generated-from contains unsupported characters" >&2
  exit 2
fi
if [[ -n "$template_commit_override" && ! "$template_commit_override" =~ ^[0-9a-f]{40}$ ]]; then
  echo "template-commit must be a 40-character lowercase Git SHA" >&2
  exit 2
fi
if [[ -z "$app_name" ]]; then
  app_name=$(printf '%s' "$project_name" | tr '[:upper:]' '[:lower:]' | tr ' ' '-')
fi
if [[ ! "$app_name" =~ ^[A-Za-z0-9][A-Za-z0-9._-]*$ ]]; then
  echo "app name contains unsupported characters" >&2
  exit 2
fi
case "$environment" in
  development|test|production) ;;
  *) echo "environment must be development, test, or production" >&2; exit 2 ;;
esac
case "$database_enabled" in
  true|false) ;;
  *) echo "database-enabled must be true or false" >&2; exit 2 ;;
esac
if [[ "$architecture" != "modular-mvc" ]]; then
  echo "architecture must be modular-mvc" >&2
  exit 2
fi

manifest="${repo_root}/.template/manifest.json"
readme="${repo_root}/README.md"
env_example="${repo_root}/.env.example"
go_mod="${repo_root}/go.mod"
for file in "$manifest" "$readme" "$env_example" "$go_mod"; do
  [[ -f "$file" ]] || { echo "required template file is missing: $file" >&2; exit 1; }
done

escape_sed() {
  printf '%s' "$1" | sed 's/[\\&|]/\\&/g'
}

replace_token() {
  local file=$1
  local token=$2
  local value=$3
  local escaped
  escaped=$(escape_sed "$value")
  if grep -Fq "$token" "$file"; then
    local temporary
    temporary=$(mktemp "${file}.setup.XXXXXX")
    sed "s|${token}|${escaped}|g" "$file" > "$temporary"
    if ! cmp -s "$file" "$temporary"; then
      mv "$temporary" "$file"
      mark_changed "${file#${repo_root}/}"
    else
      rm -f "$temporary"
    fi
  elif ! grep -Fq "$value" "$file"; then
    echo "refusing to update customized file: $file" >&2
    exit 1
  fi
}

replace_empty_manifest_field() {
  local field=$1
  local value=$2
  local escaped
  escaped=$(escape_sed "$value")
  local token="\"${field}\": \"\""
  if grep -Fq "$token" "$manifest"; then
    local temporary
    temporary=$(mktemp "${manifest}.setup.XXXXXX")
    sed "s|${token}|\"${field}\": \"${escaped}\"|" "$manifest" > "$temporary"
    mv "$temporary" "$manifest"
    mark_changed ".template/manifest.json"
  fi
}

changed_files=()
template_module="github.com/ThatSoftwareCompany/template-go-api"

mark_changed() {
  local relative_file=$1
  local existing
  for existing in "${changed_files[@]}"; do
    [[ "$existing" == "$relative_file" ]] && return
  done
  changed_files+=("$relative_file")
}

current_module=$(awk '$1 == "module" { print $2; exit }' "$go_mod")
if [[ "$current_module" != "$module_path" ]]; then
  if [[ "$current_module" != "$template_module" ]]; then
    echo "refusing to replace a non-template module path: $current_module" >&2
    exit 1
  fi
  escaped_module=$(escape_sed "$module_path")
  temporary=$(mktemp "${go_mod}.setup.XXXXXX")
  sed "s|^module .*|module ${escaped_module}|" "$go_mod" > "$temporary"
  mv "$temporary" "$go_mod"
  mark_changed "go.mod"

  escaped_template_module=$(escape_sed "$template_module")
  while IFS= read -r -d '' source_file; do
    if grep -Fq "$template_module" "$source_file"; then
      temporary=$(mktemp "${source_file}.setup.XXXXXX")
      sed "s|${escaped_template_module}|${escaped_module}|g" "$source_file" > "$temporary"
      mv "$temporary" "$source_file"
      mark_changed "${source_file#${repo_root}/}"
    fi
  done < <(find "$repo_root" -type f -name '*.go' -not -path '*/.git/*' -print0)
fi

replace_token "$manifest" "{{PROJECT_NAME}}" "$project_name"
replace_token "$manifest" "{{MODULE_PATH}}" "$module_path"
replace_token "$manifest" "{{APP_NAME}}" "$app_name"
replace_token "$manifest" "{{APP_ENV}}" "$environment"
replace_token "$manifest" "{{DATABASE_ENABLED}}" "$database_enabled"
replace_token "$manifest" "{{ARCHITECTURE}}" "$architecture"
replace_empty_manifest_field "generated_from" "$generated_from"

template_commit="$template_commit_override"
if [[ -z "$template_commit" ]] && command -v git >/dev/null 2>&1; then
  template_source=$(sed -n 's/^[[:space:]]*"template_source":[[:space:]]*"\([^"]*\)".*/\1/p' "$manifest")
  template_version=$(sed -n 's/^[[:space:]]*"template_version":[[:space:]]*"\([^"]*\)".*/\1/p' "$manifest")
  template_repository="https://github.com/${template_source}.git"
  template_commit=$(git ls-remote "$template_repository" "refs/tags/v${template_version}^{}" 2>/dev/null | awk 'NR == 1 { print $1 }')
  if [[ -z "$template_commit" ]]; then
    template_commit=$(git ls-remote "$template_repository" "refs/tags/v${template_version}" 2>/dev/null | awk 'NR == 1 { print $1 }')
  fi
fi
if [[ -n "$template_commit" ]]; then
  replace_empty_manifest_field "template_commit" "$template_commit"
else
  echo "Template release commit could not be resolved; template_commit remains empty"
fi

replace_token "$readme" "{{PROJECT_NAME}}" "$project_name"
replace_token "$readme" "{{APP_NAME}}" "$app_name"
replace_token "$env_example" "APP_NAME=template-go-api" "APP_NAME=${app_name}"
replace_token "$env_example" "APP_ENV=development" "APP_ENV=${environment}"
replace_token "$env_example" "DATABASE_ENABLED=true" "DATABASE_ENABLED=${database_enabled}"

if command -v go >/dev/null 2>&1; then
  echo "Running go mod tidy"
  go_cache=${GOCACHE:-}
  if [[ -z "$go_cache" || ! -d "$go_cache" || ! -w "$go_cache" ]]; then
    go_cache=$(mktemp -d /tmp/tsc-template-go-cache.XXXXXX)
  fi
  (cd "$repo_root" && GOCACHE="$go_cache" go mod tidy)
else
  echo "Go is not installed; skipped go mod tidy"
fi

echo "Setup completed for ${project_name}"
if [[ ${#changed_files[@]} -eq 0 ]]; then
  echo "No files changed; the setup is already applied."
else
  printf 'Modified files:\n'
  printf '  %s\n' "${changed_files[@]}"
fi
