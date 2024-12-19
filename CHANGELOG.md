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

### Changed

### Fixed

### Removed

### Fixed

### Security


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
