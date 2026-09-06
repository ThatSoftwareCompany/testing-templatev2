# Copilot instructions

Treat `AGENTS.md` as the canonical instructions for this repository. Work only on the Go backend template, preserve module boundaries, avoid secrets and unapproved dependencies, keep OpenAPI separate from controllers, and run the documented validation commands before review.

For generated repositories, preserve template-managed infrastructure and do not add product routes to `cmd/api`, `internal/platform`, or the health/errors foundation modules. Register application routes in `internal/app/routes.go` and add business modules under `internal/modules/<business-module>/`. Editing managed files is allowed only for canonical template maintenance or deliberate conflict resolution.

Authentication infrastructure is also template-managed. Do not alter the auth cookie contract, CSRF enforcement, Argon2id parameters, Ed25519 JWT validation, refresh-token rotation, or deny-by-default permission checks when adding product features. Keep tokens out of JSON responses and browser storage.

Protected product routes may use `dependencies.Auth` from `internal/app/routes.go` with explicit `auth.RequireRole` and `auth.RequirePermission` middleware. A role does not grant implicit permissions.

Supply-chain controls are template-managed too. Preserve Dependabot, dependency review, `govulncheck`, Docker Scout, `.github/security-exceptions.json`, `.template/ownership.json`, and full-SHA Action pins. Breaking template updates and semantic conflicts require explicit manual review.
