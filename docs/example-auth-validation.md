# Example authorization validation

This clean-room fixture includes one application-owned business module to
validate the authentication and authorization extension point.

## Endpoint contract

`GET /api/v1/example` is registered from `internal/app/routes.go` and handled
by `internal/modules/example`. It requires both:

- role: `example_reader`;
- permission: `example:read`.

Unauthenticated requests receive `401`. Authenticated users without the role or
permission receive `403`. A user with both receives `200` and the response
`{"status":"example route works"}`.

The fixture administrator intentionally has only the template-provided
`internal_admin` role and `errors:read` permission. This proves that an
authenticated user is not automatically authorized to access product routes.

## Local validation flow

Start the database-backed Compose stack after generating local Ed25519 keys:

```bash
./scripts/generate-dev-auth-keys.sh
docker compose up --build -d
```

Create the first administrator through the template CLI:

```bash
docker compose exec api go run ./cmd/auth -command create-admin
```

Create the fixture's `example_reader` role, `example:read` permission, and a
test user using local SQL. Keep all test credentials outside the repository.
Then exercise the following matrix with a cookie-aware HTTP client:

- CSRF issuance and rejection of missing or altered CSRF tokens;
- successful and failed login, including generic credential errors;
- administrator access to `/api/v1/internal/errors`;
- denial of the example route for the administrator without its product role;
- successful example-route access for `example_reader` with `example:read`;
- denial of the internal errors route for the product user;
- `/me` role and permission representation;
- refresh rotation, old-token reuse detection, and family revocation;
- logout, cookie clearing, and post-logout refresh rejection;
- PostgreSQL login rate limiting (`401` for the first five failures, then `429`).

The route and module are fixture-owned validation code. They must remain out of
the canonical template and must not be added to the aggregate OpenAPI document.
