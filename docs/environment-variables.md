# Rainbow Environment Variables

`rainbow` ships with some implicit defaults that can be adjusted via env variables below.

- [Configuration](#configuration)
  - [`RAINBOW_GATEWAY_DOMAINS`](#rainbow_gateway_domains)
  - [`RAINBOW_SUBDOMAIN_GATEWAY_DOMAINS`](#rainbow_subdomain_gateway_domains)
  - [`RAINBOW_TRUSTLESS_GATEWAY_DOMAINS`](#rainbow_trustless_gateway_domains)
  - [`RAINBOW_DATADIR`](#rainbow_datadir)
  - [`RAINBOW_GC_INTERVAL`](#rainbow_gc_interval)
  - [`RAINBOW_GC_THRESHOLD`](#rainbow_gc_threshold)
  - [`RAINBOW_IPNS_MAX_CACHE_TTL`](#rainbow_ipns_max_cache_ttl)
  - [`RAINBOW_PEERING`](#rainbow_peering)
  - [`RAINBOW_SEED`](#rainbow_seed)
  - [`RAINBOW_SEED_INDEX`](#rainbow_seed_index)
  - [`RAINBOW_DHT_ROUTING`](#rainbow_dht_routing)
  - [`RAINBOW_HTTP_ROUTERS`](#rainbow_http_routers)
  - [`RAINBOW_DNSLINK_RESOLVERS`](#rainbow_dnslink_resolvers)
  - [`ROUTING_IGNORE_PROVIDERS`](#routing_ignore_providers)
  - [`RAINBOW_HTTP_RETRIEVAL_ENABLE`](#rainbow_http_retrieval_enable)
  - [`RAINBOW_HTTP_RETRIEVAL_ALLOWLIST`](#rainbow_http_retrieval_allowlist)
  - [`RAINBOW_HTTP_RETRIEVAL_DENYLIST`](#rainbow_http_retrieval_denylist)
  - [`RAINBOW_HTTP_RETRIEVAL_WORKERS`](#rainbow_http_retrieval_workers)
- [Experiments](#experiments)
  - [`RAINBOW_SEED_PEERING`](#rainbow_seed_peering)
  - [`RAINBOW_SEED_PEERING_MAX_INDEX`](#rainbow_seed_peering_max_index)
  - [`RAINBOW_PEERING_SHARED_CACHE`](#rainbow_peering_shared_cache)
  - [`RAINBOW_REMOTE_BACKENDS`](#rainbow_remote_backends)
  - [`RAINBOW_REMOTE_BACKENDS_MODE`](#rainbow_remote_backends_mode)
  - [`RAINBOW_REMOTE_BACKENDS_IPNS`](#rainbow_remote_backends_ipns)
- [Logging](#logging)
  - [`GOLOG_LOG_LEVEL`](#golog_log_level)
  - [`GOLOG_LOG_FMT`](#golog_log_fmt)
  - [`GOLOG_FILE`](#golog_file)
  - [`GOLOG_TRACING_FILE`](#golog_tracing_file)
- [Testing](#testing)
  - [`GATEWAY_CONFORMANCE_TEST`](#gateway_conformance_test)
  - [`IPFS_NS_MAP`](#ipfs_ns_map)
- [Tracing](#tracing)
  - [`RAINBOW_TRACING_AUTH`](#rainbow_tracing_auth)
  - [`RAINBOW_SAMPLING_FRACTION`](#rainbow_sampling_fraction)

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

### `RAINBOW_DATADIR`

Directory for persistent data (keys, blocks, denylists)

Default: not set (uses the current directory)

### `RAINBOW_GC_INTERVAL`

The interval at which the garbage collector will be called. This is given as a string that corresponds to the duration of the interval. Set 0 to disable.

Default: `60m`

### `RAINBOW_GC_THRESHOLD`

The threshold of how much free space one wants to always have available on disk. This is used with the periodic garbage collector.

When the periodic GC runs, it checks for the total and available space on disk. If the available space is larger than the threshold, the GC is not called. Otherwise, the GC is asked to remove how many bytes necessary such that the threshold of available space on disk is met.

Default: `0.3` (always keep 30% of the disk available)

### `RAINBOW_IPNS_MAX_CACHE_TTL`

When set, it defines the upper bound limit (in ms) of how long a `/ipns/{id}`
lookup result will be cached and read from cache before checking for updates.

The limit is applied to everything under the `/ipns/` namespace, and allows to cap both
the [Time-To-Live (TTL)](https://specs.ipfs.tech/ipns/ipns-record/#ttl-uint64)
of [IPNS Records](https://specs.ipfs.tech/ipns/ipns-record/)
and the [TTL of DNS TXT records](https://datatracker.ietf.org/doc/html/rfc2181#section-8)
with [DNSLink](https://dnslink.dev/).

Default: No upper bound, [TTL from IPNS Record](https://specs.ipfs.tech/ipns/ipns-record/#ttl-uint64) or [TTL from DNSLink](https://datatracker.ietf.org/doc/html/rfc2181#section-8) used as-is.

### `RAINBOW_PEERING`

A comma-separated list of [multiaddresses](https://docs.libp2p.io/concepts/fundamentals/addressing/) of peers to stay connected to.

> [!TIP]
> If `RAINBOW_SEED` is set and `/p2p/rainbow-seed/N` value is found here, Rainbow
> will replace it with a valid `/p2p/` for a peer ID generated from same seed
> and index `N`. This is useful when `RAINBOW_SEED_PEERING` can't be used,
> or when peer routing should be skipped and specific address should be used.

Default: not set (no peering)

### `RAINBOW_SEED`

Base58 seed to derive PeerID from. Can be generated with `rainbow gen-seed`.
If set, requires `RAINBOW_SEED_INDEX` to be set as well.

Default: not set

### `RAINBOW_SEED_INDEX`

Index to derivate the PeerID identity from `RAINBOW_SEED`.

Default: not set

### `RAINBOW_DHT_ROUTING`

Control the type of Amino DHT client used for for routing. Options are `accelerated`, `standard` and `off`.

Default: `accelerated`

### `RAINBOW_HTTP_ROUTERS`

HTTP servers with /routing/v1 endpoints to use for delegated routing (comma-separated).

Default: `https://cid.contact`

### `RAINBOW_DNSLINK_RESOLVERS`

DNS-over-HTTPS servers to use for resolving DNSLink on specified TLDs (comma-separated map: `TLD:URL,TLD2:URL2`).

It is possible to override OS resolver by passing root:  `. : catch-URL`.

Default: `eth. : https://dns.eth.limo/dns-query, crypto. : https://resolver.unstoppable.io/dns-query`

### `ROUTING_IGNORE_PROVIDERS`

Comma-separated list of peer IDs whose provider records should be ignored during routing.

This is useful when you want to exclude specific peers from being considered as content providers, especially in cases where you know certain peers might advertise content but you prefer not to retrieve from them directly (for example, to ignore peer IDs from bitswap endpoints of providers that offer HTTP).

Default: not set (no peers are ignored)

### `RAINBOW_HTTP_RETRIEVAL_ENABLE`

Controls whether HTTP-based block retrieval is enabled.

When enabled, Rainbow can use [Trustless HTTP Gateways](https://specs.ipfs.tech/http-gateways/trustless-gateway/) to perform block retrievals in parallel to [Bitswap](https://specs.ipfs.tech/bitswap-protocol/). This takes advantage of peers with `/tls` + `/http` multiaddrs (HTTPS is required).

Note that this feature works in the same way as Bitswap: known HTTP-peers receive optimistic block requests even for content that they are not announcing.

Default: `false` (HTTP retrieval disabled)

### `RAINBOW_HTTP_RETRIEVAL_ALLOWLIST`

Comma-separated list of hostnames that are allowed for HTTP retrievals.

When HTTP retrieval is enabled, this setting limits HTTP retrievals to only the specified hostnames. This provides a way to restrict which gateways Rainbow will attempt to retrieve blocks from.

Example: `example.com,ipfs.example.com`

Default: not set (when HTTP retrieval is enabled, all hosts are allowed)

### `RAINBOW_HTTP_RETRIEVAL_DENYLIST`

Comma-separated list of hostnames that are allowed for HTTP retrievals.

When HTTP retrieval is enabled, this setting disables retrieval from the specified hostnames. This provides a way to restrict specific hostnames that should not be used for retrieval.

Example: `example.com,ipfs.example.com`

Default: not set (when HTTP retrieval is enabled, all no hosts are disabled)


### `RAINBOW_HTTP_RETRIEVAL_WORKERS`

The number of concurrent worker threads to use for HTTP retrievals.

This setting controls the level of parallelism for HTTP-based block retrieval operations. Higher values can improve performance when retrieving many blocks but may increase resource usage.

Default: `32`

## Experiments

### `RAINBOW_SEED_PEERING`

> [!WARNING]
> Experimental feature.

Automated version of `RAINBOW_PEERING` which does not require providing multiaddrs.

Instead, it will set up peering with peers that share the same seed (requires `RAINBOW_SEED_INDEX` to be set up).

> [!NOTE]
> Runs a separate light DHT for peer routing with the main host if DHT routing is disabled.

Default: `false` (disabled)

### `RAINBOW_SEED_PEERING_MAX_INDEX`

Informs the largest index to derive for `RAINBOW_SEED_PEERING`.
If you have more instances than the default, increase it here.

Default: 100

### `RAINBOW_PEERING_SHARED_CACHE`

> [!WARNING]
> Experimental feature, will result in increased network I/O due to Bitswap server being run in addition to the lean client.

Enable sharing of local cache to peers safe-listed with `RAINBOW_PEERING`
or `RAINBOW_SEED_PEERING`.

Once enabled, Rainbow will respond to [Bitswap](https://docs.ipfs.tech/concepts/bitswap/)
queries from these safelisted peers, serving locally cached blocks if requested.

> [!TIP]
> The main use case for this feature is scaling and load balancing across a
> fleet of rainbow, or other bitswap-capable IPFS services. Cache sharing allows
> clustered services to check if any of the other instances has a requested CID.
> This saves resources as data cached on other instance can be fetched internally
> (e.g. LAN) rather than externally (WAN, p2p).

> [!CAUTION]
> This mode comes with additional overhead, YMMV. A bitswap server
> applies `WithPeerBlockRequestFilter` and only answers to safelisted peers;
> however may still increase resource usage, as every requested CID will be
> also broadcasted to peered nodes.

Default: `false` (no cache sharing, no bitswap server, client-only)

### `RAINBOW_REMOTE_BACKENDS`

> [!WARNING]
> Experimental feature, forces setting `RAINBOW_LIBP2P=false`.

URL(s) of of remote [trustless gateways](https://docs.ipfs.tech/reference/http/gateway/#trustless-verifiable-retrieval)
to use as backend instead of libp2p node with Bitswap.

Default: not set

### `RAINBOW_REMOTE_BACKENDS_MODE`

Requires `RAINBOW_REMOTE_BACKENDS` to be set.

Controls how requests to remote backend are made.

- `block`  will use [application/vnd.ipld.raw](https://www.iana.org/assignments/media-types/application/vnd.ipld.raw) to fetch raw blocks one by one
- `car` will use [application/vnd.ipld.car](https://www.iana.org/assignments/media-types/application/vnd.ipld.car) and [IPIP-402: Partial CAR Support on Trustless Gateways](https://specs.ipfs.tech/ipips/ipip-0402/) for fetching multiple blocks per request

Default: `block`

### `RAINBOW_REMOTE_BACKENDS_IPNS`

Controls whether to fetch IPNS Records ([`application/vnd.ipfs.ipns-record`](https://www.iana.org/assignments/media-types/application/vnd.ipfs.ipns-record)) from trustless gateway defined in `RAINBOW_REMOTE_BACKENDS`.
This is done in addition to other routing systems, such as `RAINBOW_DHT_ROUTING` or `RAINBOW_HTTP_ROUTERS` (if also enabled).

Default: `true`

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
GOLOG_LOG_LEVEL="error,rainbow=debug,caboose=debug" rainbow
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

Sets the file to which the logs are saved. By default, they are printed to the standard error output.

### `GOLOG_TRACING_FILE`

Sets the file to which the tracing events are sent. By default, tracing is disabled.

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

## Tracing

See [tracing.md](tracing.md).

### `RAINBOW_TRACING_AUTH`

Optional, setting to non-empty value enables on-demand tracing per-request.

The ability to pass `Traceparent` or `Tracestate` headers is guarded by an
`Authorization` header. The value of the `Authorization` header should match
the value in the `RAINBOW_TRACING_AUTH` environment variable.

### `RAINBOW_SAMPLING_FRACTION`

Optional, set to 0 by default.

The fraction (between 0 and 1) of requests that should be sampled.
This is calculated independently of any Traceparent based sampling.

