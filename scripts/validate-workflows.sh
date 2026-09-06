#!/usr/bin/env bash

set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
repo_root=$(cd -- "${script_dir}/.." && pwd)
actionlint_version="v1.7.12"

go run "github.com/rhysd/actionlint/cmd/actionlint@${actionlint_version}" \
  "${repo_root}"/.github/workflows/*.yml

echo "GitHub Actions workflow syntax is valid"
