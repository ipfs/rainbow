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

### Removed

### Fixed

### Security

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
