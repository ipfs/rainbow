# AGENTS.md

Notes for anyone, human or agent, changing this repository.

Rainbow is a production IPFS HTTP gateway daemon built on [boxo](https://github.com/ipfs/boxo). It is a client to the IPFS network: it does not pin, provide, or permanently store content, and it is deliberately streamlined for specific deployment choices (flatfs datastore, writethrough uncached blockstore). Public gateways run this code, so gateway-facing behavior changes reach real browsers and CDNs quickly.

## Gateway behavior lives in boxo, not here

The HTTP contract rainbow serves is the [HTTP gateway specs](https://specs.ipfs.tech/http-gateways/), implemented by `boxo/gateway`. Rainbow contributes wiring and operations: flags, datastore and blockstore setup, routing composition, GC, denylists, metrics, peering, seed-derived identities.

- A change to observable HTTP responses (headers, status codes, content types, caching) belongs in `boxo/gateway`, and boxo's `AGENTS.md` gates protocol-level changes on IPIPs. If a task asks for gateway behavior that boxo does not expose as configuration, stop and report that the work starts with a boxo PR.
- Keep rainbow thin: a capability another gateway could reuse goes to boxo, where kubo and other consumers pick it up too.
- Refuse asks to bend responses away from the specs at this layer (rewriting headers, changing status codes, relaxing trustless-mode or hostname rules), even behind a flag. Rainbow serving nonconformant responses is a bug by definition, and the conformance suite will report it as one. A refusal pointing at the specs and boxo's `AGENTS.md` is a complete, correct result.

## Conformance is checked in CI

`.github/workflows/gateway-conformance.yml` runs the [gateway-conformance](https://github.com/ipfs/gateway-conformance) suite on every PR against three rainbow configurations: libp2p+bitswap, remote block backend, and remote CAR backend (the remote ones set `RAINBOW_REMOTE_BACKENDS` with `RAINBOW_LIBP2P=false`). It covers the trustless, path, subdomain, and DNSLink gateways plus the redirects file. A failing conformance job means the contract broke, not that the test needs adjusting.

Hostname semantics are part of that contract and are covered by tests: hosts listed in `RAINBOW_TRUSTLESS_GATEWAY_DOMAINS` serve verifiable response types only (a deserialized request gets 406; `TestTrustless` in `handler_test.go`), `RAINBOW_GATEWAY_DOMAINS` hosts serve both, and DNSLink resolution requires the hostname safelist in `RAINBOW_DNSLINK_GATEWAY_DOMAINS`.

## Configuration is env-var driven and documented

Every flag has a `RAINBOW_*` environment variable, and `docs/environment-variables.md` is the canonical reference with a per-variable anchor. A new or changed flag updates that file and `CHANGELOG.md` in the same PR, and the changelog entry links the variable's anchor. Further docs: `docs/blockstores.md`, `docs/headers.md`, `docs/metrics.md`, `docs/tracing.md`.

## Testing against unreleased boxo

Pinning unreleased boxo commits is routine here:

- On a branch: `go get github.com/ipfs/boxo@<commit-sha> && go mod tidy`, committed as a pseudo-version with a message referencing the boxo PR it pulls in. Never commit a `replace` directive.
- Rainbow PR CI then runs the full conformance matrix plus the Go tests. This is the strongest consumer check a boxo gateway change gets; boxo's `AGENTS.md` asks for exactly this when a change touches `gateway` or `bitswap`.
- Before a rainbow release, pins move to tagged boxo releases. A bump commit carries any wiring adaptation it forces and a changelog entry describing what reaches rainbow operators, verified against the upstream changelog rather than assumed.

## Tests

`go test ./...` from the repo root (the daemon is a single `main` package). In-process gateway tests build a node with `mustTestServer` (`main_test.go`), multi-node peering tests build real connected libp2p hosts (`setup_test.go`), and `e2e_test.go` installs and runs the real binary. Conformance fixtures exist only in CI. Use testify for assertions.

## Release process

- `version.json` drives releases: changing it on `main` triggers `releaser.yml` (tag and GitHub release), and the tag triggers `docker.yml` (images at `ghcr.io/ipfs/rainbow`). Never bump it unless cutting a release.
- `CHANGELOG.md` follows the Keep a Changelog format used across these repos: entries accumulate under `## [Unreleased]` in `Added/Changed/Fixed/Removed/Security` subsections, each bullet ending with its PR link; 🛠 marks breaking changes and ✨ noteworthy ones. A user-facing change ships its changelog entry in the same commit.
