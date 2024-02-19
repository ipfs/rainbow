# Rainbow Environment Variables

`rainbow` ships with some implicit defaults that can be adjusted via env variables below.

- [Configuration](#configuration)
  - [`RAINBOW_GATEWAY_DOMAINS`](#rainbow_gateway_domains)
  - [`RAINBOW_SUBDOMAIN_GATEWAY_DOMAINS`](#rainbow_subdomain_gateway_domains)
  - [`RAINBOW_TRUSTLESS_GATEWAY_DOMAINS`](#rainbow_trustless_gateway_domains)
  - [`RAINBOW_GC_INTERVAL`](#rainbow_gc_interval)
  - [`RAINBOW_GC_THRESHOLD`](#rainbow_gc_threshold)
  - [`KUBO_RPC_URL`](#kubo_rpc_url)
- [Logging](#logging)
  - [`GOLOG_LOG_LEVEL`](#golog_log_level)
  - [`GOLOG_LOG_FMT`](#golog_log_fmt)
  - [`GOLOG_FILE`](#golog_file)
  - [`GOLOG_TRACING_FILE`](#golog_tracing_file)
- [Testing](#testing)
  - [`GATEWAY_CONFORMANCE_TEST`](#gateway_conformance_test)
  - [`IPFS_NS_MAP`](#ipfs_ns_map)

## Configuration

### `RAINBOW_GATEWAY_DOMAINS`

Comma-separated list of [path gateway](https://specs.ipfs.tech/http-gateways/path-gateway/)
hostnames that will serve both trustless and deserialized response types.

Example:  passing `ipfs.io` will enable deserialized handler for flat
[path gateway](https://specs.ipfs.tech/http-gateways/path-gateway/)
requests with the `Host` header set to `ipfs.io`.

Default: `127.0.0.1`

### `RAINBOW_SUBDOMAIN_GATEWAY_DOMAINS`

Comma-separated list of [subdomain gateway](https://specs.ipfs.tech/http-gateways/subdomain-gateway/)
domains for website hosting with Origin-isolation per content root.

Example: passing `dweb.link` will enable handler for Origin-isolated
[subdomain gateway](https://specs.ipfs.tech/http-gateways/subdomain-gateway/)
requests with the `Host` header with subdomain values matching
`*.ipfs.dweb.link` or  `*.ipns.dweb.link`.

Default: `localhost`

### `RAINBOW_TRUSTLESS_GATEWAY_DOMAINS`

Specifies trustless-only hostnames.

Comma-separated list of [trustless gateway](https://specs.ipfs.tech/http-gateways/trustless-gateway/)
domains, where unverified website asset hosting and deserialized responses is
disabled, and **response types requested via `?format=` and `Accept` HTTP header are limited to
[verifiable content types](https://docs.ipfs.tech/reference/http/gateway/#trustless-verifiable-retrieval)**:
- [`application/vnd.ipld.raw`](https://www.iana.org/assignments/media-types/application/vnd.ipld.raw)
- [`application/vnd.ipld.car`](https://www.iana.org/assignments/media-types/application/vnd.ipld.car)
- [`application/vnd.ipfs.ipns-record`](https://www.iana.org/assignments/media-types/application/vnd.ipfs.ipns-record)

**NOTE:** This setting is applied on top of everything else, to ensure
trustless domains can't be used for phishing or direct hotlinking and hosting of third-party content. Hostnames that are passed to both `RAINBOW_GATEWAY_DOMAINS` and `RAINBOW_TRUSTLESS_GATEWAY_DOMAINS` will work only as trustless gateways. 

Example:  passing `trustless-gateway.link` will ensure only verifiable content types are supported
when request comes with the `Host` header set to `trustless-gateway.link`.

Default: none (`Host` is ignored and gateway at `127.0.0.1` supports both deserialized and verifiable response types)

## `RAINBOW_GC_INTERVAL`

The interval at which the garbage collector will be called. This is given as a string that corresponds to the duration of the interval. Set 0 to disable.


Default: `60m`

## `RAINBOW_GC_THRESHOLD`

The threshold of how much free space one wants to always have available on disk. This is used with the periodic garbage collector.

When the periodic GC runs, it checks for the total and available space on disk. If the available space is larger than the threshold, the GC is not called. Otherwise, the GC is asked to remove how many bytes necessary such that the threshold of available space on disk is met.

Default: `0.3` (always keep 30% of the disk available)

### `KUBO_RPC_URL`

Single URL or a comma separated list of RPC endpoints that provide legacy `/api/v0` from Kubo.

We use this to redirect some legacy `/api/v0` commands that need to be handled on `ipfs.io`.

**NOTE:** This is deprecated and will be removed in the future.

Default: `127.0.0.1:5001` (see `DefaultKuboRPC`)

## Logging

### `GOLOG_LOG_LEVEL`

Specifies the log-level, both globally and on a per-subsystem basis. Level can
be one of:

* `debug`
* `info`
* `warn`
* `error`
* `dpanic`
* `panic`
* `fatal`

Per-subsystem levels can be specified with `subsystem=level`.  One global level
and one or more per-subsystem levels can be specified by separating them with
commas.

Default: `error`

Example:

```console
GOLOG_LOG_LEVEL="error,bifrost-gateway=debug,caboose=debug" bifrost-gateway
```

### `GOLOG_LOG_FMT`

Specifies the log message format.  It supports the following values:

- `color` -- human readable, colorized (ANSI) output
- `nocolor` -- human readable, plain-text output.
- `json` -- structured JSON.

For example, to log structured JSON (for easier parsing):

```bash
export GOLOG_LOG_FMT="json"
```
The logging format defaults to `color` when the output is a terminal, and
`nocolor` otherwise.

### `GOLOG_FILE`

Sets the file to which the Bifrost Gateway logs. By default, the Bifrost Gateway
logs to the standard error output.

### `GOLOG_TRACING_FILE`

Sets the file to which the Bifrost Gateway sends tracing events. By default,
tracing is disabled.

Warning: Enabling tracing will likely affect performance.

## Testing

### `GATEWAY_CONFORMANCE_TEST`

Setting to `true` enables support for test fixtures required by [ipfs/gateway-conformance](https://github.com/ipfs/gateway-conformance) test suite.

### `IPFS_NS_MAP`

Adds static namesys records for deterministic tests and debugging.
Useful for testing `/ipns/` support without having to do real IPNS/DNS lookup.

Example:

```console
$ IPFS_NS_MAP="dnslink-test1.example.com:/ipfs/bafkreicysg23kiwv34eg2d7qweipxwosdo2py4ldv42nbauguluen5v6am,dnslink-test2.example.com:/ipns/dnslink-test1.example.com" ./gateway-binary
...
$ curl -is http://127.0.0.1:8081/dnslink-test2.example.com/ | grep Etag
Etag: "bafkreicysg23kiwv34eg2d7qweipxwosdo2py4ldv42nbauguluen5v6am"
```
