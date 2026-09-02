#!/usr/bin/env bash

set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
repo_root=$(cd -- "${script_dir}/.." && pwd)
temporary=$(mktemp -d /tmp/tsc-template-lifecycle.XXXXXX)
trap 'rm -rf -- "$temporary"' EXIT
source_repository="$repo_root"

for command in git go grep jq perl sed tar; do
	if ! command -v "$command" >/dev/null 2>&1; then
		echo "required command is missing: ${command}" >&2
		exit 1
	fi
done

if ! git -C "$source_repository" rev-parse v0.2.2^{commit} >/dev/null 2>&1; then
	template_source=$(jq -er '.template_source' "$repo_root/.template/manifest.json")
	source_repository="${temporary}/template-source"
	git clone -q "https://github.com/${template_source}.git" "$source_repository"
fi

v022=$(git -C "$source_repository" rev-parse v0.2.2^{commit})
v023=$(git -C "$source_repository" rev-parse v0.2.3^{commit})
v024=$(git -C "$source_repository" rev-parse v0.2.4^{commit})
v025=$(git -C "$source_repository" rev-parse v0.2.5^{commit})
source_module_path=$(awk '$1 == "module" { print $2; exit }' "$source_repository/go.mod")
if [[ -z "$source_module_path" ]]; then
	echo "unable to resolve the source Go module path" >&2
	exit 1
fi

escape_sed_replacement() {
	printf '%s' "$1" | sed 's/[\\&|]/\\&/g'
}

escaped_source_module_path=$(escape_sed_replacement "$source_module_path")

extract_tag() {
	local tag=$1
	local destination=$2
	mkdir -p "$destination"
	git -C "$source_repository" archive "$tag" | tar -x -C "$destination"
}

initialize_repository() {
	local directory=$1
	git -C "$directory" init -q
	git -C "$directory" config user.name "Template lifecycle test"
	git -C "$directory" config user.email "template-lifecycle@example.invalid"
	git -C "$directory" add .
	git -C "$directory" commit -qm "test: initialize derived repository"
}

run_setup_idempotency_test() {
	local directory="${temporary}/setup-derived"
	extract_tag v0.2.4 "$directory"
	initialize_repository "$directory"

	(
		cd "$directory"
		GOCACHE="${temporary}/setup-cache" ./scripts/setup.sh \
			--project-name "Lifecycle Test" \
			--module-path "github.com/example/lifecycle-test" \
			--app-name "lifecycle-test" \
			--environment test \
			--database-enabled false \
			--architecture modular-mvc \
			--generated-from "ThatSoftwareCompany/lifecycle-test" \
			--template-commit "$v024"
	)
	git -C "$directory" add .
	git -C "$directory" commit -qm "test: configure generated repository"
	local before
	local after
	before=$(git -C "$directory" rev-parse HEAD)
	(
		cd "$directory"
		GOCACHE="${temporary}/setup-cache" ./scripts/setup.sh \
			--project-name "Lifecycle Test" \
			--module-path "github.com/example/lifecycle-test" \
			--app-name "lifecycle-test" \
			--environment test \
			--database-enabled false \
			--architecture modular-mvc \
			--generated-from "ThatSoftwareCompany/lifecycle-test" \
			--template-commit "$v024"
	)
	after=$(git -C "$directory" rev-parse HEAD)
	[[ "$before" == "$after" ]] || { echo "setup is not idempotent" >&2; exit 1; }
	[[ ! -e "$directory/.env" ]] || { echo "setup created a real .env file" >&2; exit 1; }
	grep -Fq 'module github.com/example/lifecycle-test' "$directory/go.mod"
	jq -e '.generated_from == "ThatSoftwareCompany/lifecycle-test" and .template_commit == "'"$v024"'" and .generated_project.database_enabled == "false"' \
		"$directory/.template/manifest.json" >/dev/null

	set +e
	(
		cd "$directory"
		GOCACHE="${temporary}/setup-cache" ./scripts/setup.sh \
			--project-name "A Different Project" \
			--module-path "github.com/example/lifecycle-test" \
			--app-name "lifecycle-test" \
			--environment test \
			--database-enabled false \
			--architecture modular-mvc \
			--generated-from "ThatSoftwareCompany/lifecycle-test" \
			--template-commit "$v024"
	)
	local status=$?
	set -e
	[[ "$status" -ne 0 ]] || { echo "setup accepted an arbitrary overwrite" >&2; exit 1; }
	git -C "$directory" diff --quiet
}

run_setup_enabled_database_test() {
	local directory="${temporary}/setup-enabled-derived"
	extract_tag v0.2.4 "$directory"
	initialize_repository "$directory"
	(
		cd "$directory"
		GOCACHE="${temporary}/setup-enabled-cache" ./scripts/setup.sh \
			--project-name "Enabled Database Test" \
			--module-path "github.com/example/enabled-database-test" \
			--app-name "enabled-database-test" \
			--environment development \
			--database-enabled true \
			--architecture modular-mvc \
			--generated-from "ThatSoftwareCompany/enabled-database-test" \
			--template-commit "$v024"
	)
	jq -e '.generated_project.database_enabled == "true"' "$directory/.template/manifest.json" >/dev/null
	grep -Fq 'DATABASE_ENABLED=true' "$directory/.env.example"
}

run_update_test() {
	local directory="${temporary}/update-derived"
	extract_tag v0.2.3 "$directory"
	initialize_repository "$directory"

	go -C "$directory" mod edit -module github.com/example/update-derived
	find "$directory" -type f -name '*.go' -print0 | while IFS= read -r -d '' file; do
		sed -i "s|${escaped_source_module_path}|github.com/example/update-derived|g" "$file"
	done
	perl -0pi -e 's/func RegisterRoutes\(_ \*http\.ServeMux, _ Dependencies\) \{/func RegisterRoutes(mux *http.ServeMux, deps Dependencies) {\n\t_ = mux\n\t_ = deps\n\t\/\/ application-owned test route marker/' "$directory/internal/app/routes.go"
	git -C "$directory" add .
	git -C "$directory" commit -qm "test: add application-owned route"

	"$repo_root/scripts/template-update.sh" \
		--project-root "$directory" \
		--template-repository "$source_repository" \
		--from-commit "$v023" \
		--to-commit "$v024"
	git -C "$directory" add .
	git -C "$directory" commit -qm "test: merge v0.2.4 bridge update"
	"$repo_root/scripts/template-update.sh" \
		--project-root "$directory" \
		--template-repository "$source_repository" \
		--from-commit "$v024" \
		--to-commit "$v025"

	grep -Fq 'application-owned test route marker' "$directory/internal/app/routes.go"
	jq -e '.template_version == "0.2.5" and .template_commit == "'"$v025"'"' \
		"$directory/.template/manifest.json" >/dev/null
	if grep -RInF --exclude-dir=.git --include='*.go' --include='go.mod' "$source_module_path" "$directory"; then
		echo "derived repository retains canonical Go module imports" >&2
		exit 1
	fi
	GOCACHE="${temporary}/update-cache" go -C "$directory" test ./...
}

run_legacy_bootstrap_test() {
	local directory="${temporary}/legacy-derived"
	extract_tag v0.2.2 "$directory"
	initialize_repository "$directory"

	go -C "$directory" mod edit -module github.com/example/legacy-derived
	find "$directory" -type f -name '*.go' -print0 | while IFS= read -r -d '' file; do
		sed -i "s|${escaped_source_module_path}|github.com/example/legacy-derived|g" "$file"
	done
	git -C "$directory" add .
	git -C "$directory" commit -qm "test: customize legacy repository module"

	"$repo_root/scripts/template-update.sh" \
		--project-root "$directory" \
		--template-repository "$source_repository" \
		--from-commit "$v022" \
		--to-commit "$v023"

	test -f "$directory/internal/app/routes.go"
	grep -Fq 'github.com/example/legacy-derived/internal/platform/errstore' "$directory/internal/app/routes.go"
	if grep -RInF --exclude-dir=.git --include='*.go' --include='go.mod' "$source_module_path" "$directory"; then
		echo "legacy derived repository retains canonical Go module imports" >&2
		exit 1
	fi
	GOCACHE="${temporary}/legacy-cache" go -C "$directory" test ./...
}

expect_update_rejection() {
	local source_directory=$1
	local target_directory=$2
	local expected_message=$3
	local source_commit
	local status

	source_commit=$(git -C "$source_directory" rev-parse HEAD)
	extract_tag v0.2.4 "$target_directory"
	initialize_repository "$target_directory"
	set +e
	"$repo_root/scripts/template-update.sh" \
		--project-root "$target_directory" \
		--template-repository "$source_directory" \
		--from-commit "$v024" \
		--to-commit "$source_commit" >"${target_directory}.log" 2>&1
	status=$?
	set -e
	if [[ "$status" -eq 0 ]]; then
		echo "incompatible/deleting update was unexpectedly accepted" >&2
		cat "${target_directory}.log" >&2
		exit 1
	fi
	grep -Fq "$expected_message" "${target_directory}.log" || {
		echo "update rejection did not explain the expected reason" >&2
		cat "${target_directory}.log" >&2
		exit 1
	}
}

run_incompatible_update_test() {
	local source_directory="${temporary}/incompatible-source"
	git clone -q --no-hardlinks "$source_repository" "$source_directory"
	git -C "$source_directory" config user.name "Template lifecycle test"
	git -C "$source_directory" config user.email "template-lifecycle@example.invalid"
	git -C "$source_directory" checkout -q --detach v0.2.4
	sed -i 's/"postgresql": "16+"/"postgresql": "15+"/' "$source_directory/.template/manifest.json"
	git -C "$source_directory" add .template/manifest.json
	git -C "$source_directory" commit -qm "test: make compatibility incompatible"
	expect_update_rejection "$source_directory" "${temporary}/incompatible-target" "template compatibility changed"
}

run_deletion_update_test() {
	local source_directory="${temporary}/deleting-source"
	git clone -q --no-hardlinks "$source_repository" "$source_directory"
	git -C "$source_directory" config user.name "Template lifecycle test"
	git -C "$source_directory" config user.email "template-lifecycle@example.invalid"
	git -C "$source_directory" checkout -q --detach v0.2.4
	git -C "$source_directory" rm -q README.md
	git -C "$source_directory" commit -qm "test: delete a template file"
	expect_update_rejection "$source_directory" "${temporary}/deleting-target" "delete files require manual migration"
}

run_setup_idempotency_test
run_setup_enabled_database_test
run_update_test
run_legacy_bootstrap_test
run_incompatible_update_test
run_deletion_update_test

echo "Template lifecycle tests passed."
