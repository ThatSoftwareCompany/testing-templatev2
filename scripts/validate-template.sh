#!/usr/bin/env bash

set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
repo_root=$(cd -- "${script_dir}/.." && pwd)

required_files=(
  "LICENSE"
  "AGENTS.md"
  ".env.example"
  ".template/manifest.json"
  ".template/ownership.json"
  ".github/dependabot.yml"
  ".github/security-exceptions.json"
  ".github/workflows/template-update.yml"
  ".github/workflows/dependency-review.yml"
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
  "scripts/generate-dev-auth-keys.sh"
  "scripts/template-update.sh"
  "scripts/template-update-bootstrap.sh"
  "scripts/validate-manifest-dependencies.sh"
  "scripts/validate-workflows.sh"
  "scripts/validate-action-pins.sh"
  "scripts/validate-security-exceptions.sh"
  "scripts/check-security-exceptions.sh"
)

for relative_file in "${required_files[@]}"; do
  if [[ ! -f "${repo_root}/${relative_file}" ]]; then
    echo "required template file is missing: ${relative_file}" >&2
    exit 1
  fi
done

if [[ ! -x "${repo_root}/scripts/generate-dev-auth-keys.sh" ]]; then
  echo "development authentication key generator must be executable" >&2
  exit 1
fi

if [[ -e "${repo_root}/.env" ]]; then
  echo "a real .env file must not be committed to the template" >&2
  exit 1
fi

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
' "${repo_root}/.template/ownership.json" >/dev/null || {
  echo "template ownership metadata is invalid" >&2
  exit 1
}

template_version=$(sed -n 's/^[[:space:]]*"template_version":[[:space:]]*"\([^"]*\)".*/\1/p' "${repo_root}/.template/manifest.json")
if [[ -z "$template_version" || ! -f "${repo_root}/docs/releases/v${template_version}.md" ]]; then
  echo "release notes are missing for template version ${template_version:-unknown}" >&2
  exit 1
fi

bash -n "${repo_root}"/scripts/*.sh

(cd "$repo_root" && ./scripts/validate-action-pins.sh)
(cd "$repo_root" && ./scripts/validate-security-exceptions.sh)
(cd "$repo_root" && ./scripts/validate-workflows.sh)
(cd "$repo_root" && ./scripts/validate-manifest-dependencies.sh)

temporary_go_cache=""
go_cache=${GOCACHE:-}
if [[ -z "$go_cache" || ! -d "$go_cache" || ! -w "$go_cache" ]]; then
  temporary_go_cache=$(mktemp -d /tmp/tsc-template-go-cache.XXXXXX)
  go_cache="$temporary_go_cache"
  cleanup() {
    rm -rf -- "$temporary_go_cache"
  }
  trap cleanup EXIT
fi

(cd "$repo_root" && GOCACHE="$go_cache" go run ./cmd/template -command validate)
