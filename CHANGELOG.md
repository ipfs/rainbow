# Changelog

All notable changes to this project will be documented in this file.

Note:
* The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
* This project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## Legend
The following emojis are used to highlight certain changes:
* 🛠 - BREAKING CHANGE.  Action is required if you use this functionality.
* ✨ - Noteworthy change to be aware of.

## [Unreleased]

### Added

- Configurable routing timeouts: new options `RAINBOW_HTTP_ROUTERS_TIMEOUT` and `RAINBOW_ROUTING_TIMEOUT` (and the similar command-line flags) allow setting timeouts for routing operations. The former does it for delegated http routing requests. The latter specifies a timeout for routing requests.
- Added `BITSWAP_ENABLE_DUPLICATE_BLOCK_STATS`: Controls whether bitswap duplicate block statistics are collected. This is disabled by default since it has a performance impact.
- Allow specifying a DNSLink safelist: `RAINBOW_DNSLINK_GATEWAY_DOMAINS` defines which dnslink domains are allowed to use this gateway.

### Changed

- upgrade to `boxo` [v0.37.0](https://github.com/ipfs/boxo/releases/tag/v0.37.0)
  - include upgrade to [v0.36.0](https://github.com/ipfs/boxo/releases/tag/v0.36.0)
- upgrade to `go-ipld-prime` [v0.22.0](https://github.com/ipld/go-ipld-prime/releases/tag/v0.22.0)
- upgrade to `go-libp2p-kad-dht` [v0.38.0](https://github.com/libp2p/go-libp2p-kad-dht/releases/tag/v0.38.0)
- upgrade to `go-libp2p` [v0.47.0](https://github.com/libp2p/go-libp2p/releases/tag/v0.47.0)
- upgrade to `go-log/v2` [v2.9.1](https://github.com/ipfs/go-log/releases/tag/v2.9.1)
- upgrade to go-ds-pebble [v0.5.9](https://github.com/ipfs/go-ds-pebble/releases/tag/v0.5.9)
  - include upgrade to go-ds-pebble [v0.5.8](https://github.com/ipfs/go-ds-pebble/releases/tag/v0.5.8)
  - include upgrade to go-ds-pebble [v0.5.7](https://github.com/ipfs/go-ds-pebble/releases/tag/v0.5.7)
  - includes upgrade to pebble [v2.1.4](https://github.com/cockroachdb/pebble/releases/tag/v2.1.4)
- upgrade to `go-ds-flatfs` [v0.6.0](https://github.com/ipfs/go-ds-flatfs/releases/tag/v0.6.0)
- upgrade to badger/v4 [v4.9.1](https://github.com/dgraph-io/badger/releases/tag/v4.9.1)

### Fixed

- Upgrade go-ds-pebble to [v0.5.6](https://github.com/ipfs/go-ds-pebble/releases/tag/v0.5.6) and pebble to [v2.1.1](https://github.com/cockroachdb/pebble/releases/tag/v2.1.1)

### Removed

### Security

## [1.21.0]

### Added

### Changed

### Fixed

- Upgrade go-ds-pebble to [v0.5.6](https://github.com/ipfs/go-ds-pebble/releases/tag/v0.5.6) and pebble to [v2.1.1](https://github.com/cockroachdb/pebble/releases/tag/v2.1.1)
- Update to boxo [v0.35.1](https://github.com/ipfs/boxo/releases/tag/v0.35.1) with fixes for QUIC, httpnet and block tracing.

### Removed

### Security

## v1.20.0

### Added

- `--diagnostic-service-url` / `RAINBOW_DIAGNOSTIC_SERVICE_URL`: Configure URL for CID retrievability diagnostic service (default: `https://check.ipfs.network`). When gateway returns 504 timeout, users see "Inspect retrievability of CID" button linking to diagnostic service. Set to empty string to disable.

### Changed

- Upgrade go-ds-pebble to [v0.5.3](https://github.com/ipfs/go-ds-pebble/releases/tag/v0.5.3)


## v1.19.0

### Added

- `--max-range-request-file-size` / `RAINBOW_MAX_RANGE_REQUEST_FILE_SIZE`: Configurable limit for HTTP Range requests on large files (default: 5GiB). Range requests for files larger than this limit return HTTP 501 Not Implemented to protect against CDN issues. Specifically addresses Cloudflare's bug where range requests for files over 5GiB are silently ignored, causing the entire file to be returned instead of the requested range, leading to excess bandwidth consumption and billing.

### Changed

- Update to Boxo [v0.35.0](https://github.com/ipfs/boxo/releases/tag/v0.35.0)
- Update to go-libp2p-kad-dht [v0.35.0](https://github.com/libp2p/go-libp2p-kad-dht/releases/tag/v0.35.0)

### Fixed

- Fixed bitswap client initialization to use `time.Duration` instead of `delay.Fixed()` for rebroadcast delay, matching the updated bitswap client API


## v1.18.0

### Added

- `--bootstrap` / `RAINBOW_BOOTSTRAP`: Configure bootstrap peer multiaddrs (default: `auto`)
- AutoConf support with `auto` placeholders for bootstrap peers, DNS resolvers, and HTTP routers ([ipfs/boxo#997](https://github.com/ipfs/boxo/pull/997))
  - Configuration flags:
    - `--autoconf` / `RAINBOW_AUTOCONF`: Enable/disable automatic configuration expansion (default: `true`)
    - `--autoconf-url` / `RAINBOW_AUTOCONF_URL`: URL to fetch autoconf data from (default: `https://conf.ipfs-mainnet.org/autoconf.json`)
    - `--autoconf-refresh` / `RAINBOW_AUTOCONF_REFRESH`: Interval for refreshing autoconf data (default: `24h`)
  - When autoconf is disabled, `auto` placeholders will cause an error, requiring explicit values
- Added configurable gateway rate limiting and timeout controls via new CLI flags:
  - `--max-concurrent-requests` (env: `RAINBOW_MAX_CONCURRENT_REQUESTS`): Limits concurrent HTTP requests to protect against resource exhaustion (default: 4096). Returns 429 Too Many Requests with Retry-After header when exceeded.
  - `--retrieval-timeout` (env: `RAINBOW_RETRIEVAL_TIMEOUT`): Enforces maximum duration for content retrieval (default: 30s). Returns 504 Gateway Timeout when content cannot be retrieved within this period - both for initial retrieval (time to first byte) and between subsequent writes.

### Changed

- Default values now use `auto` placeholder that expands to IPFS Mainnet configuration via autoconf:
  - `--http-routers` default changed to `auto` (was `https://cid.contact`)
  - `--dnslink-resolvers` default changed to `. : auto` (was specific resolver list)
  - `--bootstrap` default is `auto` (new flag)
- Always upgrade pebble data format to latest [#288](https://github.com/ipfs/rainbow/pull/288)
  - This ensures:
    - Get all the latest features and improvements offered by the latest data format
    - The next pebble update is compatible with, and can upgrade, you data format
  - Possible issues:
    - Startup of new pebble version may take longer the first time.
    - Unable to revert to the previous version of pebble if the newest data format is not supported by the previous pebble. Reverting will require removing the datastore and reinitializing.
- Update to boxo/v0.34.0

### Fixed

### Removed

### Security


## [v1.17.0]

### Added

- Support for `RAINBOW_HTTP_RETRIEVAL_MAX_DONT_HAVE_ERRORS`, allows limiting
  the number of optimistic block requests performed against endpoints that
  fail to provide any of those blocks. See the [Envirionment variables
  documentation](https://github.com/ipfs/rainbow/blob/main/docs/environment-variables.md#rainbow_http_retrieval_max_dont_have_errors) for more details.
- Support for `RAINBOW_HTTP_RETRIEVAL_METRICS_LABELS_FOR_ENDPOINTS` which
  brings back the possiblity of tagging requests metrics with the endpoint the
  request is sent to. See the [Envirionment variables documentation](https://github.com/ipfs/rainbow/blob/main/docs/environment-variables.md#rainbow_http_retrieval_metrics_labels_for_endpoints) for more
  details.

### Changed

- upgrade to Boxo [v0.33.1](https://github.com/ipfs/boxo/releases/tag/v0.33.1)

### Fixed

- Fix an issue in HTTP retrievals: [#283](https://github.com/ipfs/rainbow/issues/283).
- Fix goroutine leak present in v1.16.0 (withdrawn) ([#278](https://github.com/ipfs/rainbow/issues/278)).

### Removed

### Security

## [v1.16.0]

This release has been withdrawn.

### Removed

### Security


## [v1.15.0]

### Added

### Changed

- `http-retrieval-enable` is now enabled by default. HTTP Retrieval can be disabled with
  `RAINBOW_HTTP_RETRIEVAL_ENABLE=false`. [#270](https://github.com/ipfs/rainbow/pull/270)
- upgrade to Boxo [v0.33.1-0.20250716194104-2c5f80a98e46](https://github.com/ipfs/boxo/releases/tag/v0.33.0)


### Fixed

- Setting `export RAINBOW_HTTP_RETRIEVAL_ALLOWLIST=` allowed to enable HTTP
  retrieval with an empty allowlist, so no HTTP requests would be
  performed. This is a footgun, therefore from now on this is interpreted as
  no allowlist being set. HTTP Retrieval can be disabled with
  `RAINBOW_HTTP_RETRIEVAL_ENABLE=false` instead. [#269](https://github.com/ipfs/rainbow/pull/269)
- Fix periodicGC that runs before the previous run has finished if the
  interval is too short [#273](https://github.com/ipfs/rainbow/pull/273).
- Fix issue where http retrievals silently stop (https://github.com/ipfs/boxo/issues/979).

### Removed

### Security

## [v1.14.0]

### Added

### Changed

- upgrade to Boxo [v0.32.0](https://github.com/ipfs/boxo/releases/tag/v0.32.0)

### Fixed

### Removed

### Security

## [1.13.0]

### Added

- New option `--http-retrieval-denylist`. It can be used to avoid connecting to disallowed hosts.

### Changed

- upgrade to Boxo [v0.30.0](https://github.com/ipfs/boxo/releases/tag/v0.30.0)
- upgrade go-ds-xxx packages to support `go-datastore` [v0.8.2](https://github.com/ipfs/go-datastore/releases/tag/v0.8.2) query API
- updated go-libp2p to [v0.41.1](https://github.com/libp2p/go-libp2p/releases/tag/v0.41.1)

### Fixed

- Fix exporting of routing http client metrics: the endpoint will now include `ipfs_routing_http_client_*` metrics routing clients are used. See [docs/metrics.md](https://github.com/ipfs/rainbow/blob/main/docs/metrics.md) for more details.

### Security

- This release upgrades quic-go to [v0.50.1](https://github.com/quic-go/quic-go/releases/tag/v0.50.1). It contains a fix for a remote-triggered panic.

## [1.12.0]

### Added

- HTTP block retrieval support: rainbow can now use [Trustless HTTP Gateways](https://specs.ipfs.tech/http-gateways/trustless-gateway/) to perform block retrievals in parallel to [Bitswap](https://specs.ipfs.tech/bitswap-protocol/).
  - This takes advantage of peers with `/tls` + `/http` multiaddrs (HTTPS is required).
  - You can enable HTTP retrievals with `--http-retrieval-enable`, and limit it to urls of specific hostnames with `--http-retrieval-allowlist <hostname>`.
  - You can also ignore provider records from certain peer IDs with `--routing-ignore-providers <peerID>` (for example to ignore peer IDs from bitswap endpoints of providers that offer HTTP).
  - **NOTE**: this feature works in the same way as Bitswap: known HTTP-peers receive optimistic block requests even for content that they are not announcing. See [Boxo's CHANGELOG](https://github.com/ipfs/boxo/blob/main/CHANGELOG.md) for more information.

## [1.11.0]

### Changed

- The default DNSLink resolver for `.eth` TLD changed to `https://dns.eth.limo/dns-query` and  `.crypto` one changed to `https://resolver.unstoppable.io/dns-query` [#231](https://github.com/ipfs/rainbow/pull/231)
- Upgrade to Boxo [v0.28.0](https://github.com/ipfs/boxo/releases/tag/v0.28.0)
- Upgrade go-ds-pebble to [v0.4.2](https://github.com/ipfs/go-ds-pebble/releases/tag/v0.4.2) and pebble to [v1.1.4](https://github.com/cockroachdb/pebble/releases/tag/v1.1.4)
- updated go-libp2p to [v0.40.0](https://github.com/libp2p/go-libp2p/releases/tag/v0.40.0)
- require minimum go version 1.24 in go.mod

## [1.10.1]

### Added

- Add support for custom DNSLink resolvers (e.g. to support TLDs like `.eth`, `.crypto`). It is possible to set custom DoH resolvers by setting `RAINBOW_DNSLINK_RESOLVERS` with the same convention as Kubo's [`DNS.Resolvers`](https://github.com/ipfs/kubo/blob/master/docs/config.md#dnsresolvers) ) [#224](https://github.com/ipfs/rainbow/pull/224)

## [1.10.0]

### Changed

- boxo [v0.26.0](https://github.com/ipfs/boxo/releases/tag/v0.26.0)
  - This has a number of significant updates including go-libp2p v0.38.1 and go-libp2p-kad-dht v0.28.1
- Upgrade to latest nopfs v0.14.0

## [v1.9.0]

### Added

- Added endpoints to show and purge connected peers [#194](https://github.com/ipfs/rainbow/pull/194)
- Added flags to configure bitswap/routing tuning params:
  - `routing-max-requests`
  - `routing-max-providers`
  - `routing-max-timeout`

### Changed

- boxo [v0.25.0](https://github.com/ipfs/boxo/releases/tag/v0.25.0)
- go-libp2p-kad-dht [v0.28.1](https://github.com/libp2p/go-libp2p-kad-dht/releases/tag/v0.28.1)
- passing headers that require authorization but are not authorized now results in an HTTP 401 instead of ignoring those headers
- Bitswap settings: Increased default content-discovery limits, with up to 100 in-flight requests.

## [v1.8.3]

### Changed

- updated to boxo [0.24.2](https://github.com/ipfs/boxo/releases/tag/v0.24.2)

## [v1.8.2]

### Changed

- updated go-libp2p to [v0.37.0](https://github.com/libp2p/go-libp2p/releases/tag/v0.37.0)
- require minimum go version 1.23.2 in go.mod

## [v1.8.1]

### Changed

- boxo [0.24.1](https://github.com/ipfs/boxo/releases/tag/v0.24.1)

## [v1.8.0]

### Added

- Support implicit protocol filters from [IPIP-484](https://github.com/ipfs/specs/pull/484) and customizing them via `--http-routers-filter-protocols`. [#173](https://github.com/ipfs/rainbow/pull/173)
- Dedicated [tracing docs](docs/tracing.md).

## [v1.7.0]

### Added

- Ability to specify the maximum blocksize that bitswap will replace WantHave with WantBlock responses, and to disable replacement when set to zero. [#165](https://github.com/ipfs/rainbow/pull/165)
- Support use and configuration of pebble as [datastore](https://github.com/ipfs/rainbow/blob/main/docs/blockstores.md). Pebble provides a high-performance alternative to badger. Options are available to configure key tuning parameters (`pebble-*` in `rainbow --help`).

## [v1.6.0]

### Changed

- Updated Go in go.mod to 1.22
- Updated dependencies
  - go-libp2p [0.36.3](https://github.com/libp2p/go-libp2p/releases/tag/v0.36.3)
  - go-libp2p-kad-dht [0.26.1](https://github.com/libp2p/go-libp2p-kad-dht/releases/tag/v0.26.1)
  - boxo [0.23.0](https://github.com/ipfs/boxo/releases/tag/v0.23.0)
  - badger [4.3.0](https://github.com/dgraph-io/badger/releases/tag/v4.3.0)

### Fixed

- a bug whereby `FindPeer` won't return results for peers behind NAT which only have `/p2p-circuit` multiaddrs [go-libp2p-kad-dht#976](https://github.com/libp2p/go-libp2p-kad-dht/pull/976)

## [v1.5.0]

### Added

- Simple end-to-end test to check that trustless-gateway-domains are set correctly. [#151](https://github.com/ipfs/rainbow/pull/151) [#157](https://github.com/ipfs/rainbow/pull/157)
- HTTP API to dynamically list logging subsystems and modify logging levels for subsystems. [#156](https://github.com/ipfs/rainbow/pull/156)

### Changed

- go-libp2p [0.36.1](https://github.com/libp2p/go-libp2p/releases/tag/v0.36.1)
- boxo [0.22.0](https://github.com/ipfs/boxo/releases/tag/v0.22.0)

### Fixed

- [libp2p identify agentVersion](https://github.com/libp2p/specs/blob/master/identify/README.md#agentversion) correctly indicates rainbow version when shared host is not used

## [v1.4.0]

### Added

- Tracing per request with auth header (see `RAINBOW_TRACING_AUTH`) or a fraction of requests (see `RAINBOW_SAMPLING_FRACTION`)
- Debugging with [`Rainbow-No-Blockcache`](./docs/headers.md#rainbow-no-blockcache) that is gated by the `Authorization` header and does not use the local block cache for the request

### Changed

- go-libp2p 0.35
- boxo 0.21

### Fixed

- Added more buckets to the duration histogram metric to allow for tracking operations that take longer than 1 minute.
- Release version included in `--version` output.

## [v1.3.0]

### Added

- Now supports remote backends (using RAW block or CAR requests) via `--remote-backends` (`RAINBOW_REMOTE_BACKENDS`).
- Added configurable libp2p listen addresses for the Bitswap host via the `libp2p-listen-addrs` flag and `RAINBOW_LIBP2P_LISTEN_ADDRS` environment variable

## [v1.2.2]

### Fixed

- Rainbow no longer initializes Bitswap server by default, restoring behavior from v1.0.0.

## [v1.2.1]

### Fixed

- Rainbow no longer provides announcements of blocks via Bitswap. This is not needed to provide blocks to peers with `RAINBOW_PEERING_SHARED_CACHE`.
- Rainbow no longer keeps track of other peer's Bitswap wantlists. It will only reply if they have the block at the moment. This should reduce the processing and memory usage.

## [v1.2.0]

### Added

- ✨ Now supports automatic peering with peers that have the same seed via `--seed-peering` (`RAINBOW_SEED_PEERING`). You can further read how this works in [`docs/environment-variables.md`](docs/environment-variables.md).

## [v1.1.0]

### Added

- ✨ Now supports local cache sharing with peers provided via `--peering` (`RAINBOW_PEERING`). You can further read how this works in [`docs/environment-variables.md`](docs/environment-variables.md).

## [v1.0.0]

Our first version. Check the [README](README.md) for all the information regarding 🌈 Rainbow.
