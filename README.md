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
based on [boxo](https://github.com/ipfs/boxo) which is the tooling the powers [Kubo](https://github.com/ipfs/kubo).

Rainbow uses the same Go code as the HTTP gateway in Kubo, but is fully specialized to just be a gateway:

  * Rainbow acts as DHT and Bitswap client only. Rainbow is not a server for the network.
  * Rainbow does not pin, or permanently store any content. It is just meant
    to act as gateway to content present in the network. GC strategy
  * Rainbow settings are optimized for production deployments and streamlined
    for specific choices (flatfs datastore, writethrough uncached blockstore
    etc.)
  * Denylist and denylist subscription support is included.
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

## Configuration

Rainbow can be configured via command-line arguments or environment variables.

See `rainbow --help` for information on the available options.

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

Over time, the datastore can fill up with previously fetched blocks. To free up this used disk space, garbage collection can be run. Garbage collection needs to be manually triggered. This process can also be automated by using a cron job.

By default, the API route to trigger GC is `http://$RAINBOW_CTL_LISTEN_ADDRESS/mgr/gc`. The `BytesToFree` parameter must be passed in order to specify the upper limit of how much disk space should be cleared. Setting this parameter to a very high value will GC the entire datastore.

Example cURL commmand to run GC:

    curl -v --data '{"BytesToFree": 1099511627776}' http://127.0.0.1:8091/mgr/gc

## Deployment

An ansible role to deploy Rainbow is available within the ipfs.ipfs collection in Ansible Galaxy (https://github.com/ipfs-shipyard/ansible). It includes a systemd service unit file.

Automated Docker container releases are available from the [Github container registry](https://github.com/ipfs/rainbow/pkgs/container/rainbow):

    docker pull ghcr.io/ipfs/rainbow:main-latest


## License

Dual-licensed under [MIT](https://github.com/filecoin-project/lotus/blob/master/LICENSE-MIT) + [Apache 2.0](https://github.com/filecoin-project/lotus/blob/master/LICENSE-APACHE)
