# OpenAPI documentation

OpenAPI is intentionally kept outside Go controllers. `openapi.yaml` is the active aggregate document. Module-specific contracts are maintained under `modules/` and can be reviewed independently.

- `modules/health.yaml` documents `/__ping` and `/api/v1/health`.
- `modules/errors.yaml` documents safe error schemas and the planned internal error contract. The internal route is not registered in this foundation.
- `modules/auth.yaml` records the planned authentication contract and security requirements. Authentication is not implemented in this phase.

The root document must include only routes that are actually registered. Planned routes remain marked as planned in their module document and are not aggregated as public paths until implemented.
