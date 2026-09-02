# OpenAPI documentation

OpenAPI is intentionally kept outside Go controllers. `openapi.yaml` is the active aggregate document. Module-specific contracts are maintained under `modules/` and can be reviewed independently.

- `modules/health.yaml` documents `/__ping` and `/api/v1/health`.
- `modules/errors.yaml` documents safe error schemas and the authenticated internal error contract.
- `modules/auth.yaml` documents the authentication contract and security requirements.

The root document includes only routes that are registered by the API. Internal routes remain protected by authentication and authorization middleware.
