# rainbow 

> An specialized IPFS HTTP gateway.

## About

Rainbow is an implementation of the [IPFS HTTP Gateway API](https://specs.ipfs.tech/http-gateways),
based on [boxo](https://github.com/ipfs/boxo) which is the tooling the powers [kubo](https://github.com/ipfs/kubo).

Rainbow uses the same Go code as the HTTP gateway in Kubo, but is fully specialized to just be a gateway:

  * Rainbow acts as DHT and Bitswap client only. Rainbow is not a server for the network.
  * Rainbow does not pin, or permanently store any content. It is just meant
    to act as gateway to content present in the network. GC strategy 
  * Rainbow settings are optimized for production deployments and streamlined
    for specific choices (badger datastore, writethrough uncached blockstore
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

## Deployment

An ansible role to deploy Rainbow is available within the ipfs.ipfs collection in Ansible Galaxy (https://github.com/ipfs-shipyard/ansible). It includes a systemd service unit file.

Automated Docker container releases are available from the [Github container registry](https://github.com/ipfs/rainbow/pkgs/container/rainbow):

    docker pull ghcr.io/ipfs/rainbow:main-latest


## License

Dual-licensed under [MIT](https://github.com/filecoin-project/lotus/blob/master/LICENSE-MIT) + [Apache 2.0](https://github.com/filecoin-project/lotus/blob/master/LICENSE-APACHE)
