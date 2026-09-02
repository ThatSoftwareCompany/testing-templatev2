# Agent instructions

Use `AGENTS.md` as the canonical instructions for this repository. This is the Go backend template only. Follow its architecture, security, validation, dependency, documentation, and Git review rules.

In generated repositories, do not modify template-managed infrastructure to add product features. Use `internal/app/routes.go` for route composition and add business modules under `internal/modules/<business-module>/`. Modifying template-managed files is valid only when intentionally maintaining the canonical template or resolving an update conflict while preserving both sides.
