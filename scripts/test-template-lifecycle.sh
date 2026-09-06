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
v027=$(git -C "$source_repository" rev-parse v0.2.7^{commit})
v029=$(git -C "$source_repository" rev-parse v0.2.9^{commit})
resolve_manifest_revision() {
	local version=$1
	local commit
	while IFS= read -r commit; do
		if git -C "$source_repository" show "${commit}:.template/manifest.json" 2>/dev/null \
			| jq -e --arg version "$version" '.template_version == $version' >/dev/null; then
			printf '%s' "$commit"
			return 0
		fi
	done < <(git -C "$source_repository" rev-list --all -- .template/manifest.json)
	return 1
}

v031=$(git -C "$source_repository" rev-parse --verify v0.3.1^{commit} 2>/dev/null || resolve_manifest_revision "0.3.1")
v040=$(git -C "$source_repository" rev-parse v0.4.0^{commit})
v041=$(git -C "$source_repository" rev-parse HEAD^{commit})
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

extract_revision() {
	local revision=$1
	local destination=$2
	mkdir -p "$destination"
	git -C "$source_repository" archive "$revision" | tar -x -C "$destination"
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

run_preapplied_file_update_test() {
	local directory="${temporary}/preapplied-derived"
	extract_tag v0.2.9 "$directory"
	initialize_repository "$directory"

	(
		cd "$directory"
		GOCACHE="${temporary}/preapplied-cache" go run ./cmd/template \
			-command record-provenance \
			-template-version "0.2.6" \
			-template-commit "$v027"
	)
	sed -i '1c# testing-templatev2' "$directory/README.md"
	sed -i '3cReusable Go API foundation for clean-room template validation.' "$directory/README.md"
	printf '\n\n' >> "$directory/.github/workflows/template-update.yml"
	printf '\n<!-- Application-owned lifecycle validation note. -->\n' >> "$directory/docs/template-maintenance.md"
	git -C "$directory" add .
	git -C "$directory" commit -qm "test: configure preapplied repository"

	"$repo_root/scripts/template-update.sh" \
		--project-root "$directory" \
		--template-repository "$source_repository" \
		--from-commit "$v027" \
		--to-commit "$v029"

	jq -e '.template_version == "0.2.9" and .template_commit == "'"$v029"'"' \
		"$directory/.template/manifest.json" >/dev/null
	GOCACHE="${temporary}/preapplied-cache" go -C "$directory" test ./...
	grep -Fq '# testing-templatev2' "$directory/README.md"
	grep -Fq '<!-- Application-owned lifecycle validation note. -->' "$directory/docs/template-maintenance.md"
}

run_conflicting_file_test() {
	local directory="${temporary}/conflicting-derived"
	extract_tag v0.2.7 "$directory"
	initialize_repository "$directory"

	(
		cd "$directory"
		GOCACHE="${temporary}/conflict-cache" go run ./cmd/template \
			-command record-provenance \
			-template-version "0.2.6" \
			-template-commit "$v027"
	)
	sed -i 's|github.com/ThatSoftwareCompany/template-go-api/internal/|github.com/example/conflicting-derived/internal/|' \
		"$directory/docs/template-maintenance.md"
	git -C "$directory" add .
	git -C "$directory" commit -qm "test: customize conflicting template documentation"

	set +e
	"$repo_root/scripts/template-update.sh" \
		--project-root "$directory" \
		--template-repository "$source_repository" \
		--from-commit "$v027" \
		--to-commit "$v029" >"${directory}.log" 2>&1
	local status=$?
	set -e
	[[ "$status" -ne 0 ]] || { echo "conflicting update was unexpectedly accepted" >&2; cat "${directory}.log" >&2; exit 1; }
	grep -Fq "Template update has unresolved or unapplied changes in:" "${directory}.log" || {
		echo "conflicting update did not report unresolved or unapplied changes" >&2
		cat "${directory}.log" >&2
		exit 1
	}
	grep -Fq "docs/template-maintenance.md" "${directory}.log" || {
		echo "conflicting update did not report the affected path" >&2
		cat "${directory}.log" >&2
		exit 1
	}
	grep -Fq "Resolve these files manually" "${directory}.log" || {
		echo "conflicting update did not report the manual resolution guidance" >&2
		cat "${directory}.log" >&2
		exit 1
	}
	jq -e '.template_version == "0.2.6" and .template_commit == "'"$v027"'"' \
		"$directory/.template/manifest.json" >/dev/null
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

run_v041_clean_room_update_test() {
	local directory="${temporary}/v041-derived"
	local report_file="${temporary}/v041-dry-run.md"
	extract_revision "$v031" "$directory"
	initialize_repository "$directory"

	(
		cd "$directory"
		GOCACHE="${temporary}/v041-cache" go run ./cmd/template \
			-command record-provenance \
			-template-version "0.3.1" \
			-template-commit "$v031"
	)
	go -C "$directory" mod edit -module github.com/example/v040-derived
	find "$directory" -type f -name '*.go' -print0 | while IFS= read -r -d '' file; do
		sed -i "s|${escaped_source_module_path}|github.com/example/v040-derived|g" "$file"
	done
	temporary_manifest=$(mktemp)
	jq '.dependency_versions["github.com/jackc/pgx/v5"] = "v5.8.0"' \
		"$directory/.template/manifest.json" > "$temporary_manifest"
	mv "$temporary_manifest" "$directory/.template/manifest.json"
	git -C "$directory" add .
	git -C "$directory" commit -qm "test: customize v0.4 clean-room repository"

	local before
	local after
	before=$(git -C "$directory" status --porcelain)
	"$repo_root/scripts/template-update.sh" \
		--project-root "$directory" \
		--template-repository "$source_repository" \
		--from-commit "$v031" \
		--to-commit "$v041" \
		--dry-run \
		--report-file "$report_file"
	after=$(git -C "$directory" status --porcelain)
	[[ "$before" == "$after" ]] || { echo "dry-run modified the derived repository" >&2; exit 1; }
	grep -Fq -- '- status: dry-run' "$report_file"
	grep -Fq -- '- breaking: false' "$report_file"

	"$repo_root/scripts/template-update.sh" \
		--project-root "$directory" \
		--template-repository "$source_repository" \
		--from-commit "$v031" \
		--to-commit "$v041"

	jq -e '.template_version == "0.4.1" and .template_commit == "'"$v041"'"' \
		"$directory/.template/manifest.json" >/dev/null
	jq -e '.dependency_versions["github.com/jackc/pgx/v5"] == "v5.10.0"' \
		"$directory/.template/manifest.json" >/dev/null
	test -f "$directory/.template/ownership.json"
	grep -Fq 'github.com/example/v040-derived/internal/platform/errstore' "$directory/internal/app/routes.go"
	GOCACHE="${temporary}/v041-derived-cache" go -C "$directory" test ./...
}

run_breaking_release_note_test() {
	local source_directory="${temporary}/breaking-source"
	local target_directory="${temporary}/breaking-target"
	local report_file="${temporary}/breaking-dry-run.md"
	local source_commit

	git clone -q --no-hardlinks "$source_repository" "$source_directory"
	git -C "$source_directory" config user.name "Template lifecycle test"
	git -C "$source_directory" config user.email "template-lifecycle@example.invalid"
	sed -i '0,/breaking: false/s//breaking: true/' "$source_directory/docs/releases/v0.4.0.md"
	git -C "$source_directory" add docs/releases/v0.4.0.md
	git -C "$source_directory" commit -qm "test: mark release as breaking"
	source_commit=$(git -C "$source_directory" rev-parse HEAD)

	extract_revision "$v041" "$target_directory"
	initialize_repository "$target_directory"
	(
		cd "$target_directory"
		GOCACHE="${temporary}/breaking-cache" go run ./cmd/template \
			-command record-provenance \
			-template-version "0.4.1" \
			-template-commit "$v041"
	)
	git -C "$target_directory" add .template/manifest.json
	git -C "$target_directory" commit -qm "test: record current template provenance"
	"$repo_root/scripts/template-update.sh" \
		--project-root "$target_directory" \
		--template-repository "$source_directory" \
		--from-commit "$v041" \
		--to-commit "$source_commit" \
		--dry-run \
		--report-file "$report_file"
	grep -Fq -- '- breaking: true' "$report_file"
}

run_action_pin_validation_test() {
	local directory="${temporary}/action-pins"
	mkdir -p "$directory/.github/workflows" "$directory/scripts"
	cp "$repo_root/scripts/validate-action-pins.sh" "$directory/scripts/validate-action-pins.sh"
	cat > "$directory/.github/workflows/valid.yml" <<'EOF'
name: Valid pins
on: push
jobs:
  check:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683 # v4.2.2
EOF
	(cd "$directory" && ./scripts/validate-action-pins.sh)
	sed -i 's|actions/checkout@.* # v4.2.2|actions/checkout@v4|' "$directory/.github/workflows/valid.yml"
	set +e
	(cd "$directory" && ./scripts/validate-action-pins.sh) >"${directory}.log" 2>&1
	local status=$?
	set -e
	[[ "$status" -ne 0 ]] || { echo "unpinned action was unexpectedly accepted" >&2; exit 1; }
}

run_workflow_validation_test() {
	local directory="${temporary}/workflow-validation"
	mkdir -p "$directory/.github/workflows" "$directory/scripts"
	cp "$repo_root/scripts/validate-workflows.sh" "$directory/scripts/validate-workflows.sh"

	cat > "$directory/.github/workflows/valid.yml" <<'EOF'
name: Valid workflow
on: push
jobs:
  check:
    runs-on: ubuntu-latest
    steps:
      - run: printf '%s\n' "workflow is valid"
EOF
	(
		cd "$directory"
		./scripts/validate-workflows.sh
	)

	cat > "$directory/.github/workflows/invalid.yml" <<'EOF'
name: Invalid workflow
on: push
jobs:
  check:
    runs-on: ubuntu-latest
    steps:
      - run: echo "valid block"
    invalid: [
EOF
	set +e
	(
		cd "$directory"
		./scripts/validate-workflows.sh
	) >"${directory}.log" 2>&1
	local status=$?
	set -e
	[[ "$status" -ne 0 ]] || { echo "invalid workflow was unexpectedly accepted" >&2; exit 1; }
}

run_manifest_dependency_validation_test() {
	local directory="${temporary}/manifest-dependency-validation"
	mkdir -p "$directory/.template" "$directory/scripts"
	cp "$repo_root/scripts/validate-manifest-dependencies.sh" "$directory/scripts/validate-manifest-dependencies.sh"
	cp "$repo_root/go.mod" "$directory/go.mod"
	cp "$repo_root/go.sum" "$directory/go.sum"
	jq '{dependency_versions: .dependency_versions}' \
		"$repo_root/.template/manifest.json" > "$directory/.template/manifest.json"
	(
		cd "$directory"
		./scripts/validate-manifest-dependencies.sh
	)

	sed -i 's/v5.10.0/v5.9.2/' "$directory/.template/manifest.json"
	set +e
	(
		cd "$directory"
		./scripts/validate-manifest-dependencies.sh
	) >"${directory}.log" 2>&1
	local status=$?
	set -e
	[[ "$status" -ne 0 ]] || { echo "stale manifest dependency was unexpectedly accepted" >&2; exit 1; }
}

run_security_exception_tests() {
	local directory="${temporary}/security-exceptions"
	local valid_file="${directory}/valid.json"
	local findings_file="${directory}/findings.txt"
	local invalid_file="${directory}/invalid.json"
	mkdir -p "$directory"
	cat > "$valid_file" <<'EOF'
{
  "version": 1,
  "exceptions": [
    {
      "scanner": "govulncheck",
      "id": "GO-TEST-1",
      "component": "example/module",
      "reason": "Waiting for upstream fix.",
      "owner": "platform@example.invalid",
      "issue": "https://github.com/ThatSoftwareCompany/template-go-api/issues/1",
      "expires_on": "2099-01-01"
    }
  ]
}
EOF
	printf 'GO-TEST-1\texample/module\n' > "$findings_file"
	"$repo_root/scripts/validate-security-exceptions.sh" --exceptions-file "$valid_file"
	"$repo_root/scripts/check-security-exceptions.sh" --scanner govulncheck --findings-file "$findings_file" --exceptions-file "$valid_file"
	printf 'GO-TEST-2\texample/module\n' > "$findings_file"
	set +e
	"$repo_root/scripts/check-security-exceptions.sh" --scanner govulncheck --findings-file "$findings_file" --exceptions-file "$valid_file" >/dev/null 2>&1
	local status=$?
	set -e
	[[ "$status" -ne 0 ]] || { echo "unmatched security finding was unexpectedly accepted" >&2; exit 1; }

	cat > "$invalid_file" <<'EOF'
{
  "version": 1,
  "exceptions": [
    {
      "scanner": "govulncheck",
      "id": "GO-EXPIRED",
      "component": "example/module",
      "reason": "Expired exception.",
      "owner": "platform@example.invalid",
      "issue": "https://github.com/ThatSoftwareCompany/template-go-api/issues/2",
      "expires_on": "2000-01-01"
    }
  ]
}
EOF
	set +e
	"$repo_root/scripts/validate-security-exceptions.sh" --exceptions-file "$invalid_file" >/dev/null 2>&1
	status=$?
	set -e
	[[ "$status" -ne 0 ]] || { echo "expired security exception was unexpectedly accepted" >&2; exit 1; }

	cat > "$invalid_file" <<'EOF'
{
  "version": 1,
  "exceptions": [
    {
      "scanner": "govulncheck",
      "id": "GO-INCOMPLETE",
      "component": "example/module",
      "reason": "Missing ownership and issue metadata.",
      "expires_on": "2099-01-01"
    }
  ]
}
EOF
	set +e
	"$repo_root/scripts/validate-security-exceptions.sh" --exceptions-file "$invalid_file" >/dev/null 2>&1
	status=$?
	set -e
	[[ "$status" -ne 0 ]] || { echo "incomplete security exception was unexpectedly accepted" >&2; exit 1; }
}

run_setup_idempotency_test
run_setup_enabled_database_test
run_update_test
run_preapplied_file_update_test
run_conflicting_file_test
run_legacy_bootstrap_test
run_incompatible_update_test
run_deletion_update_test
run_v041_clean_room_update_test
run_breaking_release_note_test
run_action_pin_validation_test
run_workflow_validation_test
run_manifest_dependency_validation_test
run_security_exception_tests

echo "Template lifecycle tests passed."
