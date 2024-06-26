## `Authorization`

Optional request header that guards per-request tracing and debugging features.

See [`RAINBOW_TRACING_AUTH`](./environment-variables.md#rainbow_tracing_auth)

## `Traceparent`

Optional. Clients may use this header to return a additional vendor-specific trace identification information across different distributed tracing systems.

Currently ignored unless `Authorization` matches [`RAINBOW_TRACING_AUTH`](./environment-variables.md#rainbow_tracing_auth).

> [!TIP]
> `Traceparent` value format is `00-32HEX-16HEX-2HEX`, for example:
> ```
> Traceparent: 00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-00
> base16(version) = 00 // version, currently always 00
> base16(trace-id) = 4bf92f3577b34da6a3ce929d0e0e4736
> base16(parent-id) = 00f067aa0ba902b7
> base16(trace-flags) = 00  // 00 means not sampled (manual), 01 means sampled
> ```
> 
> See [W3C Trace Context Specification](https://www.w3.org/TR/trace-context-1/#trace-context-http-headers-format) for more details.

## `Tracestate`

Optional. Clients may use this header to return a additional vendor-specific trace identification information in addition to `Traceparent`.

Currently ignored unless `Authorization` matches [`RAINBOW_TRACING_AUTH`](./environment-variables.md#rainbow_tracing_auth).

> [!TIP]
> `Tracestate` value format is 
> See [W3C Trace Context Specification](https://www.w3.org/TR/trace-context-1/#trace-context-http-headers-format).

## Rainbow-No-Blockcache

If the value is `true` the associated request will skip the local block cache and leverage a separate in-memory block cache for the request.

This header is not respected unless the request has a valid `Authorization` header

See [`RAINBOW_TRACING_AUTH`](./environment-variables.md#rainbow_tracing_auth)
