# TSC Go API Template

This repository is the canonical backend template for That Software Company. It is a Go 1.26 modular monolith template. Do not add frontend code here; the frontend template is maintained in a separate repository.

## Working agreements

- Keep code, identifiers, commit messages, and documentation in English.
- Use the standard library first. Add a dependency only when it provides clear value and is approved by the template specification.
- PostgreSQL is the default runtime dependency. The API must also support `DATABASE_ENABLED=false` for local and test scenarios.
- Keep controllers, services, repositories, clients, and DTOs separated by module. `repository` is for persistence; `client` is for external APIs.
- Keep OpenAPI documentation in `docs/openapi/`, separated from HTTP controllers and split by module.
- Never commit `.env`, credentials, tokens, passwords, private keys, or other secrets. `.env.example` may contain placeholders only.
- The repository is licensed under Apache-2.0. Do not change the declared license without repository-owner approval.
- Do not change the frontend template from this repository.

## Architecture

- `cmd/api` composes configuration, platform services, modules, and graceful shutdown.
- `internal/app/routes.go` is the application-owned route composition extension point.
- `cmd/migrate` is the explicit SQL migration CLI.
- `internal/modules/<module>` contains module transport and business responsibilities.
- `internal/platform` contains shared infrastructure only: configuration, PostgreSQL, HTTP, logging, error storage, and migrations.
- The public API is versioned under `/api/v1`.
- `/__ping` is a process liveness check and never queries PostgreSQL.
- `/api/v1/health` is a readiness check and reports PostgreSQL state without exposing failure details.

## Template-managed files and extension points

Generated repositories must keep template-managed infrastructure intact so template updates can be applied with minimal conflicts. Do not modify these paths to add product functionality:

- `cmd/api/main.go`
- `cmd/migrate/main.go`
- `internal/platform/**`
- `internal/modules/health/**`
- `internal/modules/errors/**`
- `.github/workflows/**`
- `Dockerfile`, `compose.yaml`, `.dockerignore`, and `.env.example`
- `migrations/**`, `scripts/**`, `.template/**`, and the root CI/documentation files

Add product-specific HTTP routes and module composition in `internal/app/routes.go`. Add new business modules under `internal/modules/<business-module>/` with their own controller, service, repository, client, DTO, and tests as applicable. Core liveness/readiness routes remain template-managed and must not be replaced with product routes.

The exception is intentional maintenance of the canonical template itself, including infrastructure fixes, security fixes, documentation, tests, and template lifecycle changes. When resolving an update conflict, preserve both the latest template behavior and the documented application extension point.

## Security rules

- Use `log/slog` JSON logs and never log request bodies, authorization headers, cookies, passwords, tokens, or secrets.
- Validate and return `X-Correlation-ID`; do not reflect untrusted header values without validation.
- Persist only the safe `error_events` fields defined in the migration. Never persist raw SQL errors, stack traces, request bodies, or credentials.
- CORS must use an explicit origin allowlist. Never emit `Access-Control-Allow-Origin: *` when credentials are enabled.
- Authentication is planned for a later phase. Do not expose the internal error listing endpoint before authentication and authorization exist.

## Validation commands

Run the following before opening a pull request:

```bash
test -z "$(gofmt -l .)"
go vet ./...
go test ./...
go mod tidy
go mod verify
go run ./cmd/template -command validate
go build ./cmd/api
go build ./cmd/migrate
```

Integration tests use the `integration` build tag and require PostgreSQL:

```bash
go test -tags=integration ./...
```

The template lifecycle validation also checks required files and metadata:

```bash
./scripts/validate-template.sh
```

Template update scripts must also pass shell syntax validation:

```bash
bash -n scripts/setup.sh scripts/template-update.sh
```

## Git and review

- Inspect `git status`, remotes, branch, and diffs before modifying or staging files.
- Stage explicit paths only; never use `git add .` or `git add -A`.
- Use Conventional Commits and keep commits focused.
- Feature work is reviewed through a draft PR targeting `main`; do not merge the PR from the agent.

## Code review rules

- Flag changes that introduce frontend code, unapproved dependencies, secrets, public internal endpoints, automatic production migrations, or undocumented public API changes.
- Require tests for configuration, middleware, HTTP contracts, health behavior, and persistence behavior that changes.
- Require OpenAPI and documentation updates when a public HTTP contract changes.
