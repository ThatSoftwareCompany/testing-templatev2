#!/usr/bin/env bash

set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
repo_root=$(cd -- "${script_dir}/.." && pwd)

required_files=(
  "LICENSE"
  "AGENTS.md"
  ".env.example"
  ".template/manifest.json"
  ".github/workflows/template-update.yml"
  "internal/app/routes.go"
  "cmd/auth/main.go"
  "internal/modules/auth/controller.go"
  "internal/modules/auth/middleware.go"
  "internal/modules/auth/postgres_repository.go"
  "internal/modules/auth/service.go"
  "internal/modules/auth/token.go"
  "docs/openapi/openapi.yaml"
  "docs/openapi/modules/auth.yaml"
  "docs/openapi/modules/errors.yaml"
  "migrations/000001_create_error_events.up.sql"
  "migrations/000001_create_error_events.down.sql"
  "migrations/000002_create_authentication.up.sql"
  "migrations/000002_create_authentication.down.sql"
  "scripts/template-update.sh"
)

for relative_file in "${required_files[@]}"; do
  if [[ ! -f "${repo_root}/${relative_file}" ]]; then
    echo "required template file is missing: ${relative_file}" >&2
    exit 1
  fi
done

if [[ -e "${repo_root}/.env" ]]; then
  echo "a real .env file must not be committed to the template" >&2
  exit 1
fi

template_version=$(sed -n 's/^[[:space:]]*"template_version":[[:space:]]*"\([^"]*\)".*/\1/p' "${repo_root}/.template/manifest.json")
if [[ -z "$template_version" || ! -f "${repo_root}/docs/releases/v${template_version}.md" ]]; then
  echo "release notes are missing for template version ${template_version:-unknown}" >&2
  exit 1
fi

bash -n "${repo_root}"/scripts/*.sh

go_cache=${GOCACHE:-}
if [[ -z "$go_cache" || ! -d "$go_cache" || ! -w "$go_cache" ]]; then
  go_cache=$(mktemp -d /tmp/tsc-template-go-cache.XXXXXX)
fi

(cd "$repo_root" && GOCACHE="$go_cache" go run ./cmd/template -command validate)
