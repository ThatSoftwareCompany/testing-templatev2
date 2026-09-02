# Template maintenance

The manifest is the source of truth for the template identity, version, source repository, compatibility, and generated origin.

## Implemented lifecycle safeguards

- `scripts/setup.sh` resolves `template_commit` from the published source tag and never uses the generated repository's own commit.
- `scripts/setup.sh` records `generated_from` from the derived repository identifier or module path.
- `cmd/template -command validate` validates the manifest contract.
- `scripts/validate-template.sh` verifies required template files and blocks a real `.env` file.
- CI runs the lifecycle validation and writes build outputs outside the repository root.
- `scripts/template-update.sh` applies a normalized direct patch when possible and falls back to a three-way patch between recorded and target template commits.
- The updater imports source base blobs before a three-way patch, recognizes safely pre-applied files, and reports unapplied paths instead of accepting a partial update.
- `.github/workflows/template-update.yml` detects version tags and opens derived-repository PRs with least-privilege write permissions.
- The workflow accepts an optional `TEMPLATE_UPDATE_TOKEN` secret for updates that modify `.github/workflows` files.
- Compatibility changes are rejected automatically; file deletions remain a manual migration.

## Update workflow

The derived-repository workflow performs these steps:

1. Detect the newest `vMAJOR.MINOR.PATCH` tag from `ThatSoftwareCompany/template-go-api`.
2. Compare the generated repository's recorded `template_commit` and compatibility fields. The workflow uses a valid recorded source commit first and falls back to the version tag only when the recorded commit cannot be resolved in the template repository.
3. Apply a normalized direct patch when the generated repository matches the update context; otherwise apply a three-way patch from the recorded commit to the tagged commit.
4. Normalize the canonical Go module path to the generated repository's module path in Go changes without changing three-way merge context.
5. Refuse incompatible Go/PostgreSQL changes and template file deletions.
6. Refuse partial patches and report unresolved or unapplied paths.
7. Record the new `template_version` and `template_commit` only after a complete update.
8. Create an update pull request in the derived repository.
9. Leave application-specific conflicts for manual resolution; it must never merge generated pull requests automatically.

If a generated repository contains its own Git commit in `template_commit`, the workflow resolves the source commit from the matching release tag and opens a provenance-repair pull request.

Repositories generated from versions before `v0.2.4` require a sequential bridge update before consuming `v0.2.5` or newer. The pre-`v0.2.4` updater could not normalize Go module paths in newly added files. If a direct update reports `no required module provides package github.com/ThatSoftwareCompany/template-go-api/internal/...`, close that update PR, apply `v0.2.4` manually with the current updater, merge it, and then run the automatic update again. The lifecycle suite exercises this bridge as `v0.2.3 -> v0.2.4 -> v0.2.5`; the recommended current target is the latest release because the immutable `v0.2.6`, `v0.2.7`, and `v0.2.8` tags predate the final lifecycle workflow correction.

Repositories that already applied the lifecycle code but still use the pre-`v0.2.9` workflow detector require a one-time bootstrap. Create a small reviewed PR that updates `.github/workflows/template-update.yml` to the current template version while preserving the repository's `TEMPLATE_UPDATE_TOKEN` configuration. Merge that bootstrap PR, then run the automatic updater again. Do not resolve a full historical update by copying template-managed files over application changes.

The workflow requires GitHub Actions to be allowed to create pull requests in the derived repository. It is skipped when running in the canonical template repository itself.

## File ownership and extension points

Template-managed files contain reusable runtime, security, CI, Docker, migration, and lifecycle behavior. Generated repositories should not modify them to add product functionality. This includes `cmd/api`, `internal/platform`, `internal/modules/health`, `internal/modules/errors`, `.github/workflows`, `scripts`, `.template`, and the root Docker, migration, and CI files.

Product-specific code belongs in new business modules under `internal/modules/<business-module>/`. Register those modules in `internal/app/routes.go`, which is an application-owned extension point intentionally preserved by `scripts/template-update.sh` once it exists. Repositories generated before this extension point was introduced receive the file during their first compatible update; subsequent updates preserve its contents. The template-provided `/__ping` and `/api/v1/health` routes are operational routes and remain active automatically; they do not need to be copied or re-registered by the generated project.

The exception is maintenance of the canonical template itself. Template maintainers may change managed files when implementing a deliberate template, security, test, documentation, or lifecycle change, with the corresponding version, validation, and review updates.

## Authentication release migration

`v0.3.0` adds template-managed authentication infrastructure and the second SQL migration. A derived repository consuming this release must apply migrations before starting a database-backed API. It must also provide `AUTH_PRIVATE_KEY_FILE`, `AUTH_PUBLIC_KEY_FILE`, `AUTH_KEY_ID`, `AUTH_JWT_ISSUER`, `AUTH_JWT_AUDIENCE`, and an `AUTH_CSRF_SECRET` containing at least 32 bytes. Generate local development keys with `scripts/generate-dev-auth-keys.sh`; production keys and secrets must be mounted outside the repository.

The release changes the internal errors route from documented-only to registered and protected. Preserve its `RequirePermission(..., "errors:read", ...)` boundary. Do not expose it directly or add implicit administrator permissions. Existing derived applications should review cookie, CORS, migration, and environment changes in the generated update PR before merging.

## Derived repository onboarding checklist

Complete this checklist after generating a repository from the template and before running the scheduled or manual update workflow:

- [ ] Create a dedicated fine-grained personal access token or GitHub App installation token scoped to the derived repository.
- [ ] Grant Contents: Read and write, Workflows: Read and write, and Pull requests: Read and write.
- [ ] Add the token as the repository Actions secret `TEMPLATE_UPDATE_TOKEN` under `Settings -> Secrets and variables -> Actions`.
- [ ] In `Settings -> Actions -> General`, enable read and write workflow permissions.
- [ ] Enable GitHub Actions pull request creation if the organization policy exposes that option.
- [ ] Run the workflow manually and verify that it creates an update branch and pull request.

The template cannot complete updates that modify `.github/workflows` with only the built-in `GITHUB_TOKEN`. Configure `TEMPLATE_UPDATE_TOKEN` before the first update to avoid a push error related to missing `workflows` permission.

### Workflow update token details

The built-in `GITHUB_TOKEN` can create branches and pull requests, but GitHub rejects pushes that create or modify files under `.github/workflows` unless the token has the special Workflows repository permission. Each derived repository should configure an Actions repository secret named `TEMPLATE_UPDATE_TOKEN` before enabling automatic template updates.

Use a dedicated fine-grained personal access token or GitHub App installation token scoped to the derived repository with:

- Contents: Read and write
- Workflows: Read and write
- Pull requests: Read and write

The workflow falls back to `GITHUB_TOKEN` when the secret is absent, which is sufficient only for updates that do not modify workflow files. Never commit the token or place it in `.env` files.

If an update reports a conflict, inspect `git status` and the paths printed by the updater. Keep the latest template behavior and preserve intentional application changes; do not copy the entire template over the derived repository. For `.github/workflows/template-update.yml`, keep the latest detection/provenance logic together with the `TEMPLATE_UPDATE_TOKEN` checkout and `GH_TOKEN` settings. After resolving, stage the files, run `go test ./...`, `go vet ./...`, and `git diff --check`, then record provenance with `go run ./cmd/template -command record-provenance -template-version <version> -template-commit <commit>` before committing the reviewed update.

## Compatibility policy

- Patch and compatible minor template updates may be proposed automatically when the declared Go and PostgreSQL compatibility ranges remain valid.
- Changes to public routes, configuration semantics, database migrations, dependency major versions, or security behavior require an explicit compatibility note.
- Breaking changes require a new documented template version and a migration note for derived repositories.

## Post-merge repository checklist

After the backend and separate frontend PRs are reviewed and merged:

- [x] Mark `ThatSoftwareCompany/template-go-api` as a GitHub Template Repository in `Settings -> General -> Template repository`.
- [ ] Mark the frontend template repository as a GitHub Template Repository in `Settings -> General -> Template repository`.
- [ ] Confirm that template generation preserves the manifest and setup-script behavior.
