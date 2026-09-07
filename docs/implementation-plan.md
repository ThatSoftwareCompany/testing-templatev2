# Go API Template Implementation Plan

This plan is the release roadmap for the backend template only. The frontend template is a separate repository and is not changed by this work.

## Baseline and release policy

The template is a Go 1.26 modular monolith with PostgreSQL enabled by default. `DATABASE_ENABLED=false` remains an explicit supported mode for local development, tests, and liveness-only deployments. The minimum compatible Go version remains `1.26.0`; CI and Docker use the latest validated patch in the 1.26 line, initially `1.26.7`.

Every release must have a version tag, a release note under `docs/releases/`, and a validated `.template/manifest.json`. Release notes declare `breaking: true|false`, summarize additions and fixes, identify incompatible changes, and document migration instructions. Template updates are applied through reviewed pull requests and never merged automatically.

## Completed releases

- `0.2.3`: onboarding, ownership rules, `internal/app/routes.go`, AI guidance, and derived-repository update documentation.
- `0.2.4`: update lifecycle and module-path/provenance hotfixes.
- `0.2.5`: foundation hardening, PostgreSQL integration, Docker smoke tests, setup idempotency, and lifecycle coverage.
- `0.2.6`: lifecycle module-normalization hotfix and legacy bridge coverage.
- `0.2.7`: lifecycle fix commit published before the release metadata preparation was completed; retained as an immutable historical tag.
- `0.2.8`: corrected publication of the lifecycle hotfix with consistent version metadata.
- `0.2.9`: provenance detection hotfix for derived repositories whose recorded source commit differs from an immutable historical tag.
- `0.2.10`: updater hardening for pre-applied files, custom module paths, base blobs, and explicit conflict reporting.
- `0.3.1`: reconciliation release for protected application-route composition merged after the original `v0.3.0` tag.

## `0.2.5` — foundation hardening

This is the foundation hardening baseline. It closes the initial quality gap without introducing authentication or other feature-level breaking changes.

- Expand unit and HTTP coverage for configuration, middleware, error responses, CORS, security headers, logging behavior, and panic recovery.
- Run PostgreSQL integration tests for pool initialization, migrations, safe error persistence, filters, limits, idempotency, and unavailable-database failures.
- Test `scripts/setup.sh` for idempotency, custom module paths, enabled/disabled database configuration, overwrite protection, and absence of real `.env` files.
- Test template updates with temporary repositories, including `v0.2.2 -> v0.2.3`, `v0.2.3 -> v0.2.4`, custom module paths, preservation of `internal/app/routes.go`, old-repository bootstrap, and rejection of incompatible or deleting updates.
- Add Docker build and smoke coverage for development and production images, non-root execution, PostgreSQL readiness, `/__ping`, `/api/v1/health`, and no-database execution.
- Make CI enforce `go mod tidy` without a diff, `go mod verify`, formatting, vet, normal tests, race tests, builds, shell validation, manifest validation, and PostgreSQL integration.
- Keep the coverage goal behavioral: critical branches and operational scenarios must be exercised; an arbitrary 100% line threshold is not required.

Acceptance criteria: the complete foundation/lifecycle matrix passes locally and in CI, the updater preserves application-owned routes, Docker smoke tests pass, and the generated repository can run both with and without PostgreSQL.

## `0.2.8` — corrected lifecycle release

The `v0.2.7` tag points to the merged lifecycle fix, but it was created before the release metadata preparation was completed and therefore contains `template_version: 0.2.6`. It is retained unchanged. `v0.2.8` is the corrected release and the recommended update target.

- Publish consistent manifest and release-note metadata for the lifecycle fix.
- Keep lifecycle fixtures independent of optional developer tools and global Git identity configuration.
- Preserve custom Go module paths and application-owned routes during derived-repository updates.

Acceptance criteria: `v0.2.8` points to the corrected release commit, template CI passes, and `testing-template` updates from its current `v0.2.6` state through a reviewed automatic pull request.

## `0.2.9` — provenance detection hotfix

The lifecycle workflow now prefers a valid `template_commit` that can be resolved in the source repository and falls back to the version tag only when the recorded commit is unavailable. This prevents an immutable historical tag from replaying changes that are already present in a derived repository.

- Publish the workflow correction as a release instead of relying on an untagged `main` commit.
- Document the one-time workflow bootstrap required by repositories that already applied the lifecycle fix with the older detector.
- Keep the update path reviewed, explicit, and free of force-pushed tags.

Acceptance criteria: `v0.2.9` includes the corrected workflow and source CI passes. Repositories with partially applied historical changes are migrated through the `v0.2.10` updater hardening path.

## `0.2.10` — updater conflict hardening

The updater must distinguish clean generated repositories, repositories with harmless pre-applied template files, and repositories with semantic application changes.

- Import source base blobs before attempting a three-way merge.
- Normalize generated Go module paths without corrupting patch context.
- Skip already-applied files when comparison is safe, including harmless YAML formatting differences.
- Detect failed hunks even when Git returns a successful process status after partial application.
- Report unresolved or unapplied paths and leave provenance unchanged until the update completes successfully.
- Preserve application-owned `internal/app/routes.go` and never resolve semantic conflicts automatically.
- Exercise clean-room, legacy, pre-applied, custom-module, route-preservation, incompatible, deletion, and explicit-conflict scenarios.

Acceptance criteria: the lifecycle suite passes, the actual `testing-template` history can be repaired with a reviewed migration, and a clean `testing-templatev2` repository can consume the release without historical conflicts.

## `0.3.0` — authentication and authorization (current implementation)

Add the documented auth contract:

- `GET /api/v1/auth/csrf`
- `POST /api/v1/auth/login`
- `POST /api/v1/auth/refresh`
- `POST /api/v1/auth/logout`
- `GET /api/v1/auth/me`
- `GET /api/v1/internal/errors?endpoint=<path>`

There is no public registration, password recovery, or Google OAuth in this release. Login is administrated initially.

- Access JWT: Ed25519/EdDSA, approximately 15 minutes, with strict algorithm, issuer, audience, subject, expiration, not-before, and key-ID validation.
- Refresh token: opaque cryptographically random value, stored only as a hash in PostgreSQL, with family tracking, rotation, revocation, and reuse detection; initial lifetime approximately 30 days.
- Both tokens use HttpOnly cookies. `Secure=false` and `SameSite=Lax` apply in development/test; `Secure=true` and `SameSite=Strict` apply in production. Cookies have no `Domain` attribute.
- Signed double-submit CSRF protection uses `X-CSRF-Token`; mutable authenticated operations require it.
- Add `users`, `roles`, `permissions`, `user_roles`, `role_permissions`, `refresh_tokens`, and `login_attempts` migrations. Passwords are 15–128 characters, without artificial composition rules, and use Argon2id with benchmarked initial parameters of 64 MiB, 3 iterations, and parallelism 1.
- Login rate limiting is distributed through PostgreSQL and login failures use generic responses to prevent account enumeration.
- PEM-mounted Ed25519 keys use `AUTH_PRIVATE_KEY_FILE`, `AUTH_PUBLIC_KEY_FILE`, `AUTH_KEY_ID`, `AUTH_JWT_ISSUER`, and `AUTH_JWT_AUDIENCE`; signed CSRF tokens use `AUTH_CSRF_SECRET`. `AUTH_ACCESS_TOKEN_TTL` defaults to `15m` and `AUTH_REFRESH_TOKEN_TTL` defaults to `720h`, constrained to 7–30 days.
- Authorization is deny-by-default. The internal error endpoint requires the explicit `errors:read` permission.

Implementation acceptance criteria: the auth module is isolated under `internal/modules/auth`; database-backed startup fails fast for missing or invalid auth configuration; database-disabled startup remains available with auth endpoints returning `503`; the admin CLI creates the initial user and grants only the explicit `internal_admin` role and `errors:read` permission; all session tokens remain in HttpOnly cookies; application-owned routes can consume `app.Dependencies.Auth` and compose explicit role and permission guards; and unit, HTTP, and PostgreSQL integration coverage exercises the security contract.

### Clean-room authentication validation

`testing-templatev2` is the clean-room fixture for the generated-project flow. It owns a single `internal/modules/example` business module and registers `GET /api/v1/example` only from `internal/app/routes.go`. The endpoint requires both `example_reader` and `example:read`; the fixture administrator is intentionally denied while the product reader is allowed. The fixture validation must cover no session (`401`), an authenticated user without the role or permission (`403`), an authorized user (`200`), CSRF failures, generic login failures, refresh rotation and reuse detection, family revocation, logout, `/me`, protected internal errors, rate limiting, PostgreSQL integration, and `DATABASE_ENABLED=false` startup. This product route and its OpenAPI contract must remain outside the canonical template.

## `0.4.0` — supply-chain security and lifecycle

- Add weekly grouped Dependabot updates for Go modules and GitHub Actions; major updates remain separate for review.
- Add blocking Dependency Review for high and critical findings, exact-version `govulncheck`, strict `go.sum` verification, and full-SHA pinning for every Action.
- Scan the local production image with Docker Scout and block fixable high and critical findings. The CI workflow requires read-only `DOCKER_SCOUT_HUB_USER` and `DOCKER_SCOUT_HUB_PASSWORD` Actions secrets.
- Add `.github/security-exceptions.json` with exact matching, ownership, issue tracking, and expiry validation; exceptions never bypass Action pinning.
- Add `.template/ownership.json`, updater dry-run mode, release-note parsing, explicit reports, pre-applied-file detection, compatibility checks, and manual conflict guidance.
- Mark breaking updates in generated PRs and fail the dedicated review gate until the derived application is migrated manually.
- Keep artifact attestations as an evaluation/documentation item, not an implementation requirement for this release.

Acceptance criteria: the template validates with all Action pins immutable; Dependency Review, `govulncheck`, Docker Scout, module integrity, lifecycle, race, integration, and build gates pass; dry-run leaves repositories unchanged; clean-room updates preserve `internal/app/routes.go`; and breaking, conflicting, incompatible, deleting, expired-exception, and unpinned-action scenarios fail safely.

## `0.4.1` and `0.4.2` — update metadata compatibility

- `0.4.1` synchronizes template-managed dependency metadata when a derived repository update changes `go.mod`.
- `0.4.2` adds `scripts/template-update-bootstrap.sh` and runs it before the normal updater so repositories generated from the current template can migrate stale manifest metadata safely.
- The bootstrap preserves provenance, generated project fields, and application-owned paths. Repositories generated before this workflow exists require one reviewed bootstrap commit during their first migration.

Acceptance criteria: stale dependency metadata is repaired without changing provenance, dry-run leaves the repository unchanged, invalid target commits fail without partial edits, and the lifecycle suite preserves `internal/app/routes.go` and generated project metadata.

## `0.5.0` — provider-agnostic same-origin deployment contract

Document and validate the deployment contract without changing the frontend template:

- frontend at `/`, API at `/api/v1`, and liveness at `/__ping`;
- TLS terminated by a reverse proxy;
- Secure cookies and normally unnecessary cross-origin CORS in production;
- migrations run as an explicit job;
- graceful shutdown and readiness checks;
- trusted `X-Forwarded-*` handling from configured proxies;
- GitHub Actions deployments use OIDC instead of long-lived cloud credentials.

## `1.0.0` — final validation

Use the public `testing-templatev2` clean-room repository created from the GitHub Template Repository and run the full matrix: new setup, PostgreSQL mode, no-database mode, a business route registered only through `internal/app/routes.go`, tests, Docker, migrations, CI, automatic updates, route preservation, authentication, authorized internal errors, and secret/file absence checks. Publish `1.0.0` only after that repository passes from a clean start.

## Template maintenance and repository checklist

Derived repositories need `TEMPLATE_UPDATE_TOKEN` with repository-scoped Contents, Workflows, and Pull requests read/write permissions, and GitHub Actions must be allowed to create pull requests. Application code belongs in modules under `internal/modules/` and is registered in `internal/app/routes.go`; template-managed operational files should remain unchanged unless intentionally modifying the template itself.

The update mechanism includes version detection, automatic PR creation in derived repositories, compatibility checks, a breaking-change log, and manual resolution of minimal application conflicts. `testing-template` remains the legacy regression fixture; `testing-templatev2` remains the clean-room acceptance fixture. After review and merge, mark both backend and frontend repositories as GitHub Template Repositories under `Settings -> General -> Template repository`.

## Explicit non-goals for the current backend release

Google OAuth, public registration, password recovery, frontend implementation, and cloud-provider-specific deployment remain outside this release. The repository license is Apache-2.0. The next release focuses on supply-chain security and lifecycle improvements; same-origin deployment remains planned for `0.5.0`.
