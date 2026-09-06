#!/usr/bin/env bash

set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
repo_root=$(cd -- "${script_dir}/.." && pwd)
manifest_dependency_mismatches=0
direct_modules=$(cd "$repo_root" && go list -m -f '{{if not .Indirect}}{{.Path}}{{"\t"}}{{.Version}}{{end}}' all)

while IFS=$'\t' read -r module version; do
	[[ -z "$module" || -z "$version" ]] && continue
	manifest_version=$(jq -er --arg module "$module" '.dependency_versions[$module] // empty' \
		"${repo_root}/.template/manifest.json" 2>/dev/null || true)
	if [[ -z "$manifest_version" ]]; then
		continue
	elif [[ "$manifest_version" != "$version" ]]; then
		echo "template manifest dependency mismatch: ${module} manifest=${manifest_version} go.mod=${version}" >&2
		manifest_dependency_mismatches=$((manifest_dependency_mismatches + 1))
	fi
done <<< "$direct_modules"

if [[ "$manifest_dependency_mismatches" -ne 0 ]]; then
	exit 1
fi

echo "template manifest dependencies match direct Go modules"
