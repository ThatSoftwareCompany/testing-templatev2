# Copilot instructions

Treat `AGENTS.md` as the canonical instructions for this repository. Work only on the Go backend template, preserve module boundaries, avoid secrets and unapproved dependencies, keep OpenAPI separate from controllers, and run the documented validation commands before review.

For generated repositories, preserve template-managed infrastructure and do not add product routes to `cmd/api`, `internal/platform`, or the health/errors foundation modules. Register application routes in `internal/app/routes.go` and add business modules under `internal/modules/<business-module>/`. Editing managed files is allowed only for canonical template maintenance or deliberate conflict resolution.
