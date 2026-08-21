# API Versioning

The public control-plane contract is versioned in the URL. The current stable API is `/v1`; health, readiness, and metrics endpoints are unversioned because they are operational probes rather than workload contracts.

Every `/v1` response includes `API-Version: v1`. `GET /v1/version` exposes machine-readable API/server versions, supported agent range, and supported/expected database schema versions. The router registers v1 routes through one version boundary so a future version can be implemented without changing v1 handlers in place.

## Compatibility Rules

A backward-compatible change may add an optional request field, add a response field, add an endpoint, add an enum value where clients are required to tolerate unknown values, or improve behavior without changing documented semantics. Clients must ignore unknown JSON response fields.

A breaking change includes removing or renaming a field or endpoint, changing a field type or meaning, making an optional field required, narrowing accepted input, changing authentication requirements, changing a stable error code for the same condition, or changing namespace isolation behavior. Breaking changes require a new URL version.

A deprecated field or endpoint remains functional for the supported lifetime of its API version. Deprecated endpoints return `Deprecation: true`; when known, they also return an HTTP-date `Sunset` header and a `Link` with `rel="deprecation"` pointing to migration guidance. Deprecation must be reflected in OpenAPI and release notes.

A removed endpoint is never silently removed from a supported version. It is removed only in a new major API version or after the entire old version reaches end of support.

## Introducing v2

A future `/v2` must have a separate route registration boundary, OpenAPI contract, compatibility tests, client methods, and migration guide. Shared domain logic may be reused, but v2 request/response DTOs must not be aliases that accidentally change v1 JSON. Both versions should run side-by-side during migration. `/v1` remains the default used by existing CLI and agent releases until their compatibility window closes.

No `/v2` routes exist today; requests to them return `404`.

## Verification

Compatibility tests assert that selected v1 routes retain their status and response envelope, every v1 route exposes its version header, v2 is not accidentally registered, and deprecation metadata uses standard response headers. The canonical contract is `api/openapi.yaml`.
