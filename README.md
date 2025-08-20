<h1 align="center">
  <br>
  <a href="https://github.com/ipfs/rainbow/assets/157609/fd1bed0f-2055-468e-93e7-0aea158aa953"><img src="https://github.com/ipfs/rainbow/assets/157609/8bf5b727-a360-4906-b965-826823c37aa3" alt="Rainbo logo" title="Rainbow logo" width="200"></a>
  <br>
  Rainbow
  <br>
</h1>

<p align="center" style="font-size: 1.2rem;">A to-be-released production-grade IPFS HTTP Gateway written in Go (using <a href="https://github.com/ipfs/boxo">Boxo</a>).</p>

<p align="center">
  <a href="https://ipfs.tech"><img src="https://img.shields.io/badge/project-IPFS-blue.svg?style=flat-square" alt="Official Part of IPFS Project"></a>
  <a href="https://discuss.ipfs.tech"><img alt="Discourse Forum" src="https://img.shields.io/discourse/posts?server=https%3A%2F%2Fdiscuss.ipfs.tech"></a>
  <a href="https://matrix.to/#/#ipfs-space:ipfs.io"><img alt="Matrix" src="https://img.shields.io/matrix/ipfs-space%3Aipfs.io?server_fqdn=matrix.org"></a>
  <a href="https://github.com/ipfs/rainbow/actions"><img src="https://img.shields.io/github/actions/workflow/status/ipfs/rainbow/go-test.yml?branch=main" alt="ci"></a>
  <a href="https://codecov.io/gh/ipfs/rainbow"><img src="https://codecov.io/gh/ipfs/rainbow/branch/main/graph/badge.svg?token=9eG7d8fbCB" alt="coverage"></a>
  <a href="https://github.com/ipfs/rainbow/releases"><img alt="GitHub release" src="https://img.shields.io/github/v/release/ipfs/rainbow?filter=!*rc*"></a>
  <a href="https://godoc.org/github.com/ipfs/rainbow"><img src="https://img.shields.io/badge/godoc-reference-5272B4.svg?style=flat-square" alt="godoc reference"></a>
</p>

<hr />

## About

Rainbow is an implementation of the [IPFS HTTP Gateway API](https://specs.ipfs.tech/http-gateways),
based on [boxo](https://github.com/ipfs/boxo) which is the tooling that powers [Kubo](https://github.com/ipfs/kubo) IPFS implementation.
It uses the same Go code as the [HTTP gateway](https://specs.ipfs.tech/http-gateways/) in Kubo,
but is fully specialized to just be a gateway:

  * Rainbow acts as [Amino DHT](https://blog.ipfs.tech/2023-09-amino-refactoring/)
    and [Bitswap](https://specs.ipfs.tech/bitswap-protocol/) client only.
  * Rainbow does not pin, or permanently store any content. It is just meant
    to act as gateway to content present in the network.
  * Rainbow settings are optimized for production deployments and streamlined
    for specific choices (flatfs datastore, writethrough uncached blockstore
    etc.)
  * [Denylist](https://specs.ipfs.tech/compact-denylist-format/) and denylist subscription support is included.
  * And more to come...


## Building

```
go build
```

## Running

```
rainbow
```

Use `rainbow --help` for documentation.

### Docker

Automated Docker container releases are available from the [Github container registry](https://github.com/ipfs/rainbow/pkgs/container/rainbow):

- 🟢 Releases
  - `latest` always points at the latest stable release
  - `vN.N.N` point at a specific [release tag](https://github.com/ipfs/rainbow/releases)
- 🟠 Unreleased developer builds
  - `main-latest` always points at the `HEAD` of the `main` branch
  - `main-YYYY-DD-MM-GITSHA` points at a specific commit from the `main` branch
- ⚠️ Experimental, unstable builds
  - `staging-latest` always points at the `HEAD` of the `staging` branch
  - `staging-YYYY-DD-MM-GITSHA` points at a specific commit from the `staging` branch
  - This tag is used by developers for internal testing, not intended for end users

When using Docker, make sure to pass necessary config via `-e`:
```console
$ docker pull ghcr.io/ipfs/rainbow:main-latest
$ docker run --rm -it --net=host -e RAINBOW_SUBDOMAIN_GATEWAY_DOMAINS=dweb.link ghcr.io/ipfs/rainbow:main-latest
```

See [`/docs/environment-variables.md`](./docs/environment-variables.md).


## Configuration

### CLI and Environment Variables

Rainbow can be configured via command-line arguments or environment variables.

See `rainbow --help` and [`/docs/environment-variables.md`](./docs/environment-variables.md) for information on the available options.

Rainbow uses a `--datadir` (or `RAINBOW_DATADIR` environment variable) as
location for persisted data. It defaults to the folder in which `rainbow` is
run.

### Peer Identity

**Using a key file**: By default generates a `libp2p.key` in its data folder if none exist yet. This
file stores the libp2p peer identity.

**Using a seed + index**: Alternatively, random can be initialized with a
32-byte, b58 encoded seed and a derivation index. This allows to use the same
seed for multiple instances of rainbow, and only change the derivation index.

The seed and index can be provided as command line arguments or environment
vars (`--seed` , `--seed-index`). The seed can also be provided as a `seed`
file in the datadir folder. A new random seed can be generated with:

    rainbow gen-seed > seed

To facilitate the use of rainbow with systemd
[`LoadCredential=`](https://www.freedesktop.org/software/systemd/man/systemd.exec.html#LoadCredential=ID:PATH)
directive, we look for both `libp2p.key` and `seed` in
`$CREDENTIALS_DIRECTORY` first.

### AutoConf

Rainbow supports automatic configuration of bootstrap peers, DNS resolvers, and HTTP routing endpoints through the autoconf feature. When enabled (default), Rainbow will fetch configuration from a remote URL and automatically expand the special `auto` placeholder value with network-appropriate defaults.

This feature can be configured via:
- `--autoconf` / `RAINBOW_AUTOCONF`: Enable/disable autoconf (default: `true`)
- `--autoconf-url` / `RAINBOW_AUTOCONF_URL`: URL to fetch configuration from (default: `https://conf.ipfs-mainnet.org/autoconf.json`)
- `--autoconf-refresh` / `RAINBOW_AUTOCONF_REFRESH`: How often to refresh configuration (default: `24h`)

The `auto` placeholder can be used in:
- `--bootstrap` / `RAINBOW_BOOTSTRAP`: Bootstrap peer multiaddrs
- `--http-routers` / `RAINBOW_HTTP_ROUTERS`: HTTP routing endpoints
- `--dnslink-resolvers` / `RAINBOW_DNSLINK_RESOLVERS`: DNS-over-HTTPS resolvers

**Note:** When autoconf is disabled (`--autoconf=false`), using the `auto` placeholder will cause an error. You must provide explicit values for these configurations when autoconf is disabled.

### Denylists

Rainbow can subscribe to append-only denylists using the `--denylists` flag. The value is a comma-separated list of URLs to subscribe to, for example: `https://denyli.st/badbits.deny`. This will download and update the denylist automatically when it is updated with new entries.

Denylists can be manually placed in the `$RAINBOW_DATADIR/denylists` folder too.

See [NoPFS](https://github.com/ipfs-shipyard/nopfs) for an explanation of the denylist format. Note that denylists should only be appended to while Rainbow is running. Editing differently, or adding new denylist files, should be done with Rainbow stopped.

## Blockstores

Rainbow ships with a number of possible blockstores for the purposes of caching data locally.
Because Rainbow, as a gateway-only IPFS implementation, is not designed for long-term data storage there are no long
term guarantees of support for any particular backing data storage.

See [Blockstores](./docs/blockstores.md) for more details.

## Garbage Collection

Over time, the datastore can fill up with previously fetched blocks. To free up this used disk space, garbage collection can be run. 

### Automatic Garbage Collection

Automatic GC is enabled by default, and configurable with [`RAINBOW_GC_INTERVAL`](https://github.com/ipfs/rainbow/blob/main/docs/environment-variables.md#rainbow_gc_interval) and [`RAINBOW_GC_THRESHOLD`](https://github.com/ipfs/rainbow/blob/main/docs/environment-variables.md#rainbow_gc_threshold).

### Manual Garbage Collection

Garbage collection can also be manually triggered. This process can be automated by using a cron job.

The API route to trigger GC is `http://$RAINBOW_CTL_LISTEN_ADDRESS/mgr/gc`. The `BytesToFree` parameter must be passed as JSON in the POST request body to specify the upper limit of how much disk space should be cleared. GC will try to clear as much space as needed, up to `BytesToFree`, to create `RAINBOW_GC_THRESHOLD` of free space. Setting this parameter to a very high value will GC the entire datastore.

Example cURL commmand to run GC:

    curl -v --data '{"BytesToFree": 1099511627776}' http://127.0.0.1:8091/mgr/gc

## Logging

While the logging can be controlled via [environment variable](./docs/environment-variables.md#logging) it is also
possible to dynamically modify the logging at runtime.

- `http://$RAINBOW_CTL_LISTEN_ADDRESS/mgr/log/level?subsystem=<system name or * for all system>&level=<level>` will set the logging level for a subsystem
- `http://$RAINBOW_CTL_LISTEN_ADDRESS/mgr/log/ls` will return a comma separated list of available logging subsystems

## Purging Peer Connections

Connections to a specific peer, or to all peers, can be closed and the peer information removed from the peer store. This can be useful to help determine if the presence/absence of a connection to a peer is affecting behavior. Be aware that purging a connection is inherently racey as it is possible for the peer to reestablish a connection at any time following a purge.

If `RAINBOW_DHT_SHARED_HOST=false` this endpoint will not show peers connected to DHT host, and only list ones used for Bitswap.

- `http://$RAINBOW_CTL_LISTEN_ADDRESS/mgr/purge?peer=<peer_id>` purges connection and info for peer identifid by peer_id
- `http://$RAINBOW_CTL_LISTEN_ADDRESS/mgr/purge?peer=all` purges connections and info for all peers
- `http://$RAINBOW_CTL_LISTEN_ADDRESS/mgr/peers` returns a list of currently connected peers

Example cURL commmand to show connected peers and purge peer connection:

    curl http://127.0.0.1:8091/mgr/peers
    curl http://127.0.0.1:8091/mgr/purge?peer=QmQzqxhK82kAmKvARFZSkUVS6fo9sySaiogAnx5EnZ6ZmC

## Tracing

See [docs/tracing.md](docs/tracing.md).

## Deployment

Suggested method for self-hosting is to run a [prebuilt Docker image](#docker).

An ansible role to deploy Rainbow is available within the ipfs.ipfs collection in Ansible Galaxy (https://github.com/ipfs-shipyard/ansible). It includes a systemd service unit file.

## Release

1. Create a PR from branch `release-vX.Y.Z` against `main` that:
   1. Tidies the [`CHANGELOG.md`](CHANGELOG.md) with the changes for the current release
   2. Updates the  [`version.json`](./version.json) file
2. Once the release checker creates a draft release, copy-paste the changelog into the draft
3. Merge the PR, the release will be automatically created once the PR is merged

## License

Dual-licensed under [MIT](https://github.com/filecoin-project/lotus/blob/master/LICENSE-MIT) + [Apache 2.0](https://github.com/filecoin-project/lotus/blob/master/LICENSE-APACHE)
