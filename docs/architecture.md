# Architecture

The template is a modular monolith. Each business module owns its HTTP controller, service rules, DTOs, persistence repository, and external clients when those responsibilities exist.

```text
HTTP middleware -> module controller -> module service -> repository/client
```

## Composition

- `cmd/api` owns startup, signal handling, dependency composition, and shutdown.
- `internal/app/routes.go` is the application-owned composition extension point for product modules.
- `app.Dependencies.Auth` exposes the template authentication service to application-owned route composition.
- `cmd/migrate` owns explicit schema migration commands.
- `internal/platform/config` validates environment configuration.
- `internal/platform/db` owns the `pgxpool` lifecycle.
- `internal/platform/httpserver` owns shared middleware and response contracts.
- `internal/platform/logging` creates the JSON `slog` logger.
- `internal/platform/errstore` owns safe PostgreSQL/no-op error persistence.
- `internal/platform/migrate` wraps `golang-migrate` SQL migrations.
- `internal/modules/health` owns liveness/readiness transport and rules.
- `internal/modules/auth` owns administrator provisioning, password verification, session tokens, CSRF validation, and authorization middleware.
- `internal/modules/errors` owns the safe error listing use case and is exposed only through authenticated `errors:read` authorization.

## Ownership boundaries

The template owns operational infrastructure, including `cmd/api`, `internal/platform`, `internal/modules/health`, and `internal/modules/errors`. The template registers `/__ping` and `/api/v1/health` itself; generated projects should not add product routes to those files.

Generated projects own `internal/app/routes.go` and new business modules under `internal/modules/<business-module>/`. `internal/app/routes.go` receives shared dependencies, including the template auth service, and is the place where a project registers its module routes. Protected product routes should compose `auth.RequireRole` and `auth.RequirePermission`; neither role names nor authentication alone grant access. The template updater preserves this file so product route composition remains local to the generated repository.

## Database modes

PostgreSQL is enabled by default and required through `DATABASE_URL`. Set `DATABASE_ENABLED=false` when the API must run without PostgreSQL; in that mode health reports `database: "disabled"` and error persistence is no-op.

## Public and internal boundaries

The public API is versioned under `/api/v1`. `/__ping` intentionally sits outside the API version because it is an infrastructure liveness probe. The internal error listing route is registered under `/api/v1/internal/errors` only after authentication and the explicit `errors:read` permission check.

## Authentication boundary

Authentication is a self-contained module. `cmd/api` supplies its repository and key configuration, while controllers expose only the HTTP contract. Passwords use Argon2id; access tokens are short-lived Ed25519/EdDSA JWTs; refresh tokens are opaque, hashed in PostgreSQL, rotated, revoked by family, and checked for reuse. All session cookies are HttpOnly and never include a `Domain` attribute. CSRF uses a signed double-submit cookie and `X-CSRF-Token`.

The API requires the Ed25519 PEM files, JWT metadata, and a CSRF secret when PostgreSQL is enabled. Generate development keys with `scripts/generate-dev-auth-keys.sh`; production keys must come from a secret manager or protected mounted volume. With `DATABASE_ENABLED=false`, auth endpoints return `503` and no key files are required.
