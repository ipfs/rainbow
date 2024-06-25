## `Authorization`

Optional request header that guards per-request tracing features.

See [`RAINBOW_TRACING_AUTH`](./environment-variables.md#rainbow_tracing_auth)

## `Traceparent`

See [`RAINBOW_TRACING_AUTH`](./environment-variables.md#rainbow_tracing_auth)

## `Tracestate`

See [`RAINBOW_TRACING_AUTH`](./environment-variables.md#rainbow_tracing_auth)

## Rainbow-No-Blockcache

If the value is `true` the associated request will skip the local block cache and leverage a separate in-memory block cache for the request.

This header is not respected unless the request has a valid `Authorization` header
