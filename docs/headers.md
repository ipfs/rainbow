## `Authorization`

Optional request header that guards per-request tracing and debugging features.

See [`RAINBOW_TRACING_AUTH`](./environment-variables.md#rainbow_tracing_auth)

## `Traceparent`

Optional. Clients may use this header to return a additional vendor-specific trace identification information across different distributed tracing systems.

Currently ignored unless `Authorization` matches [`RAINBOW_TRACING_AUTH`](./environment-variables.md#rainbow_tracing_auth).

> [!TIP]
> `Traceparent` value format can be found in [W3C Trace Context Specification](https://www.w3.org/TR/trace-context-1/#trace-context-http-headers-format).
>
> Sample code for generating it can be found in [boxo/docs/tracing.md](https://github.com/ipfs/boxo/blob/main/docs/tracing.md#generate-traceparent-header)

## `Tracestate`

Optional. Clients may use this header to return a additional vendor-specific trace identification information in addition to `Traceparent`.

Currently ignored unless `Authorization` matches [`RAINBOW_TRACING_AUTH`](./environment-variables.md#rainbow_tracing_auth).

> [!TIP]
> `Tracestate` value format can be found in [W3C Trace Context Specification](https://www.w3.org/TR/trace-context-1/#trace-context-http-headers-format).

## Rainbow-No-Blockcache

If the value is `true` the associated request will skip the local block cache and leverage a separate in-memory block cache for the request.

This header is not respected unless the request has a valid `Authorization` header

See [`RAINBOW_TRACING_AUTH`](./environment-variables.md#rainbow_tracing_auth)
