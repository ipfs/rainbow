# Rainbow Environment Variables

`rainbow` ships with some implicit defaults that can be adjusted via env variables below.

- [Configuration](#configuration)
  - [`RAINBOW_GATEWAY_DOMAINS`](#rainbow_gateway_domains)
  - [`RAINBOW_SUBDOMAIN_GATEWAY_DOMAINS`](#rainbow_subdomain_gateway_domains)
  - [`RAINBOW_TRUSTLESS_GATEWAY_DOMAINS`](#rainbow_trustless_gateway_domains)
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

Comma-separated list of path gateway hostnames. For example, passing `ipfs.io` will enable handler for standard [path gateway](https://specs.ipfs.tech/http-gateways/path-gateway/) requests with the `Host` header set to `ipfs.io`.

Default: `127.0.0.1`

### `RAINBOW_SUBDOMAIN_GATEWAY_DOMAINS`

Comma-separated list of [subdomain gateway](https://specs.ipfs.tech/http-gateways/subdomain-gateway/) domains. For example, passing `dweb.link` will enable handler for standard [subdomain gateway](https://specs.ipfs.tech/http-gateways/subdomain-gateway/) requests with the `Host` header set to `*.ipfs.dweb.link` and  `*.ipns.dweb.link`.

Default: `localhost`

### `RAINBOW_TRUSTLESS_GATEWAY_DOMAINS`

Comma-separated list of [trustless gateway](https://specs.ipfs.tech/http-gateways/trustless-gateway/) domains. These gateways can also be included in [`RAINBOW_SUBDOMAIN_GATEWAY_DOMAINS`](#rainbow_subdomain_gateway_domains), which means they will be trustless subdomain gateways.

Default: none

### `KUBO_RPC_URL`

Default: `127.0.0.1:5001` (see `DefaultKuboRPC`)

Single URL or a comma separated list of RPC endpoints that provide legacy `/api/v0` from Kubo.

We use this to redirect some legacy `/api/v0` commands that need to be handled on `ipfs.io`.

This is deprecated and will be removed in the future.

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
