# AI agent guidance

`AGENTS.md` is the canonical repository instruction file for Codex and other coding agents. `CLAUDE.md`, `GEMINI.md`, and `.github/copilot-instructions.md` should point contributors back to the same rules.

Agents must inspect Git status, remotes, branches, and existing files before modifying the repository. They must preserve unrelated work, keep backend and frontend repositories separate, avoid secrets, preserve the declared Apache-2.0 license, run relevant validation, and leave public API/OpenAPI documentation synchronized.

Authentication is template-managed infrastructure. Agents must preserve its migrations, cookie contract, CSRF enforcement, Argon2id password parameters, Ed25519/EdDSA JWT validation, refresh-token rotation, and deny-by-default permission boundary. Product routes belong in `internal/app/routes.go` and new business modules, not in the auth or operational composition files. Protected product routes should use `dependencies.Auth` with explicit `auth.RequireRole` and `auth.RequirePermission` middleware and test `401`, `403`, and authorized `2xx` behavior. Product-only routes must not be added to the canonical aggregate OpenAPI document.

Supply-chain infrastructure is also template-managed. Preserve `.github/dependabot.yml`, dependency review, `govulncheck`, Docker Scout scanning, `.github/security-exceptions.json`, `.template/ownership.json`, and SHA-pinned workflow references. A breaking template update is reviewed and migrated manually; agents must never bypass a failing security gate or resolve semantic update conflicts by copying the entire template.
