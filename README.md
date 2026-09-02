# testing-templatev2

Reusable Go API foundation for That Software Company. The generated application name is `testing-templatev2`.

## Requirements

- Go 1.26.0 or newer. CI and Docker currently validate Go 1.26.7.
- PostgreSQL 16 or newer for database-backed execution.
- Docker and Docker Compose for the container workflow.

## Local execution

PostgreSQL is enabled by default and `DATABASE_URL` is required in that mode. Copy `.env.example` to a local `.env` outside version control, set the database values, and export them before starting:

```bash
set -a
source .env
set +a
go run ./cmd/api
```

To run the API without PostgreSQL:

```bash
DATABASE_ENABLED=false go run ./cmd/api
```

The application validates all environment variables at startup. `APP_ENV` must be `development`, `test`, or `production`.

## Docker Compose

Set the local-only `POSTGRES_USER`, `POSTGRES_PASSWORD`, and `POSTGRES_DB` variables in your shell or a local `.env` file, then run:

```bash
docker compose up --build
```

Compose starts PostgreSQL with a healthcheck, runs development migrations on API startup, and stores PostgreSQL data in a named local volume. The production image is built separately:

```bash
docker build --target production -t example-api:local .
```

The production container runs as a non-root user. Production migrations must be executed explicitly with the `migrate` binary or `go run ./cmd/migrate`; automatic startup migrations are rejected in `production`.

## Migrations

```bash
go run ./cmd/migrate -command up
go run ./cmd/migrate -command down -steps 1
go run ./cmd/migrate -command version
```

The migration directory uses normal `.up.sql` and `.down.sql` files. The current foundation creates `error_events`.

## Operational endpoints

- `GET /__ping` checks only that the process is alive. It never queries PostgreSQL.
- `GET /api/v1/health` checks readiness. It reports `database: "disabled"`, `"up"`, or `"down"`; the latter returns HTTP 503 without exposing failure details.

Every response includes a validated `X-Correlation-ID`. Error responses include the same value in the JSON error object.

## Architecture

This is a modular monolith with a simple MVC flow:

```text
routes -> controller -> service -> repository/client
```

- `internal/modules/health` owns health transport and readiness rules.
- `internal/modules/errors` prepares the future internal error use case.
- `internal/app/routes.go` is the application-owned extension point for registering product modules.
- `internal/platform` owns configuration, HTTP, logging, PostgreSQL, migrations, and safe error storage.
- `repository` is reserved for persistence.
- `client` is reserved for external APIs.
- Public routes are versioned with `/api/v1`.
- OpenAPI files live in `docs/openapi/` and are separate from controllers.

The template owns the operational composition in `cmd/api`, including `/__ping` and `/api/v1/health`. A generated project must not add product routes to those files or to `internal/modules/health`. Add product modules under `internal/modules/<business-module>/` and register them from `internal/app/routes.go`; the template updater preserves that extension point.

## Security

The foundation emits structured JSON logs through `log/slog`, security headers, an explicit CORS allowlist, and correlation IDs. It never logs request bodies, authorization headers, cookies, passwords, tokens, or secrets. Persisted HTTP 5xx events contain only the safe fields documented by the `error_events` migration.

In production, frontend and backend should be served under the same public origin. CORS credentials are enabled only for explicitly allowed origins; `*` is never accepted.

The internal error listing route is intentionally not registered until authentication and authorization exist.

## Tests and quality checks

```bash
go mod tidy
go mod verify
test -z "$(gofmt -l .)"
go vet ./...
go test ./...
go test -race ./...
go build ./cmd/api
go build ./cmd/migrate
go build ./cmd/template

# Template lifecycle and shell checks
bash -n scripts/*.sh
./scripts/test-template-lifecycle.sh
```

Integration tests require PostgreSQL and use the `integration` build tag:

```bash
TEST_DATABASE_URL='postgres://USER:PASSWORD@localhost:5432/DB?sslmode=disable' go test -tags=integration ./...
```

The CI workflow runs the integration suite against PostgreSQL 16 and also performs Docker build and smoke checks. The hardening release measures critical behavior and scenarios rather than requiring an arbitrary 100% line coverage threshold.

## Setup script

The Bash setup script safely configures a generated project without arbitrary overwrites:

```bash
./scripts/setup.sh \
  --project-name "Example API" \
  --module-path "github.com/example/example-api" \
  --app-name "example-api" \
  --environment development \
  --database-enabled true \
  --architecture modular-mvc \
  --generated-from "ThatSoftwareCompany/example-api"
```

The setup script resolves the source commit for the published `template_version` tag and records it in `template_commit`; `--template-commit` can provide an explicit override. It never uses the generated repository's own commit as template provenance. It records `generated_from` from `--generated-from`, defaulting to the generated module path, and preserves both values on subsequent idempotent runs.

Validate the template manifest and required files with:

```bash
./scripts/validate-template.sh
```

## Template metadata and updates

`.template/manifest.json` records the source repository, template version, template commit, generated origin, compatibility, dependencies, and update policy. The update automation detects new template versions, opens PRs in derived repositories, enforces compatibility, and leaves breaking-change records and application-specific conflicts for manual review.

The generated repository also includes a scheduled and manually dispatchable template-update workflow. It looks for `vMAJOR.MINOR.PATCH` tags, applies a three-way patch from the recorded `template_commit`, normalizes the canonical Go module path to the generated repository's module path, checks Go and PostgreSQL compatibility, records new provenance, and opens a pull request. It never merges automatically. The repository owner must allow GitHub Actions to create pull requests and review generated changes manually.

If an older generated project recorded its own repository commit instead of the template commit, the workflow resolves provenance from the matching release tag and opens a small repair pull request automatically.

### Required derived-repository onboarding

Complete these steps immediately after generating a repository from this template and before running the template-update workflow:

1. Create a dedicated fine-grained personal access token or GitHub App installation token. Scope it to the generated repository only and grant:
   - Contents: Read and write
   - Workflows: Read and write
   - Pull requests: Read and write
2. Add it to the generated repository under `Settings -> Secrets and variables -> Actions` as the repository secret `TEMPLATE_UPDATE_TOKEN`.
3. In `Settings -> Actions -> General`, allow read and write workflow permissions and allow GitHub Actions to create pull requests when the organization policy exposes that option.
4. Run `Template update` through `Actions -> Template update -> Run workflow` once and verify that it can create its update branch and pull request.

GitHub's built-in `GITHUB_TOKEN` is retained as a fallback for updates that do not modify workflow files, but it is not sufficient for the full template lifecycle. Without `TEMPLATE_UPDATE_TOKEN`, a workflow update can fail with `refusing to allow a GitHub App to create or update workflow ... without workflows permission`. Never commit the token or place it in `.env` files. If the organization requires approval for fine-grained tokens, the token must be approved before it can write to the generated repository.

Token creation reference: [GitHub fine-grained personal access tokens](https://github.com/settings/personal-access-tokens/new).

If an update reports a conflict in `.github/workflows/template-update.yml`, preserve the template's latest provenance and update logic together with the `TEMPLATE_UPDATE_TOKEN` checkout and `GH_TOKEN` configuration. Run the generated repository tests before committing the manually resolved update.

The template maintainer must publish version tags such as `v0.1.0` before derived repositories can detect releases. The initial release tag should point to the merged template commit.

## Planned phases

The current foundation release is `0.2.9`, which includes the `0.2.5` hardening work, lifecycle module-normalization and legacy bridge fixes, and provenance detection for immutable release tags. The immutable `v0.2.6`, `v0.2.7`, and `v0.2.8` tags were created before the final lifecycle workflow correction; new generated repositories should use `v0.2.9`. The next planned releases are:

- `0.3.0`: administrated login, Argon2id, Ed25519/EdDSA JWTs, approximately 15-minute access tokens, 30-day rotating/revocable refresh tokens, HttpOnly cookies, environment-specific Secure and SameSite policies, CSRF protection, authentication/authorization middleware, and authorized access to `/api/v1/internal/errors?endpoint=<path>`.
- `0.4.0`: Dependabot, dependency review, `govulncheck`, Docker image scanning, strict `go.sum` checks, full-SHA Actions pinning, release notes, and safer updater conflict reporting.
- `0.5.0`: provider-agnostic same-origin deployment contract and trusted reverse-proxy configuration.
- `1.0.0`: final validation from a clean `testing-templatev2` repository.

Google OAuth, public registration, password recovery, and frontend implementation are not part of the current backend foundation.

After review and merge, the backend and frontend repositories must be marked as GitHub Template Repositories from `Settings -> General -> Template repository`. This is a post-merge checklist item, not an automated repository mutation.

## License

This template is distributed under the [Apache License 2.0](LICENSE).
